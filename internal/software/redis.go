//go:build linux

package software

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

const redisConfPath = "/etc/redis/redis.conf"
const redisStatePath = "/var/lib/siroc/redis.json"

func (m *Manager) Redis() (*rpc.RedisSettings, error) {
	return GetRedis()
}

func (m *Manager) SetRedis(in rpc.RedisSettings) error {
	return ApplyRedis(in)
}

func redisVersions() []string {
	return []string{"7.0", "7.2", "7.4", "8.0", "8.2"}
}

func redisInstalled() (bool, string) {
	if p, err := exec.LookPath("redis-server"); err == nil {
		out, _ := exec.Command(p, "--version").CombinedOutput()
		line := strings.TrimSpace(string(out))
		if line != "" {
			return true, line
		}
		return true, "redis-server"
	}
	if ok, ver := dpkgVersion("redis-server"); ok {
		return true, ver
	}
	return dpkgVersion("redis")
}

func installRedis(version string) error {
	version, err := pickVersion(redisVersions(), version, "7.2")
	if err != nil {
		return err
	}
	_ = exec.Command("systemctl", "stop", "redis-server").Run()
	if err := aptInstall("build-essential", "pkg-config", "libssl-dev", "wget", "tar", "autoconf", "libjemalloc-dev"); err != nil {
		return err
	}
	if err := buildRedis(version); err != nil {
		if err2 := installRedisApt(version); err2 != nil {
			return fmt.Errorf("redis %s: %v; apt fallback: %w", version, err, err2)
		}
	}
	if err := os.MkdirAll("/etc/redis", 0755); err != nil {
		return err
	}
	if _, err := os.Stat(redisConfPath); err != nil {
		if err := os.WriteFile(redisConfPath, []byte(defaultRedisConf()), 0640); err != nil {
			return err
		}
	}
	_ = exec.Command("chown", "redis:redis", redisConfPath).Run()
	if b, err := os.ReadFile(redisConfPath); err == nil {
		conf := setRedisLine(string(b), "daemonize", "no")
		conf = setRedisLine(conf, "supervised", "no")
		_ = os.WriteFile(redisConfPath, []byte(conf), 0640)
		_ = exec.Command("chown", "redis:redis", redisConfPath).Run()
	}
	if err := writeRedisUnit(); err != nil {
		return err
	}
	_ = os.MkdirAll("/var/lib/redis", 0750)
	_ = os.MkdirAll("/var/log/redis", 0755)
	if _, err := os.Stat("/var/lib/redis/dump.rdb"); err == nil {
		_ = os.Rename("/var/lib/redis/dump.rdb", "/var/lib/redis/dump.rdb.bak")
	}
	if exec.Command("id", "redis").Run() != nil {
		_ = exec.Command("useradd", "--system", "--home-dir", "/var/lib/redis", "--shell", "/usr/sbin/nologin", "redis").Run()
	}
	_ = exec.Command("chown", "-R", "redis:redis", "/var/lib/redis", "/etc/redis", "/var/log/redis").Run()
	_ = exec.Command("systemctl", "daemon-reload").Run()
	_ = exec.Command("systemctl", "enable", "--now", "redis-server").Run()
	st, _ := GetRedis()
	if st != nil {
		_ = ApplyRedis(*st)
	}
	return nil
}

func installRedisApt(version string) error {
	_, code := osRelease()
	if version == "7.0" {
		removeRepo("redis")
	} else {
		if err := writeKeyring("https://packages.redis.io/gpg", "/etc/apt/keyrings/cp-redis.gpg"); err != nil {
			return err
		}
		if err := writeRepo("redis", fmt.Sprintf("deb [signed-by=/etc/apt/keyrings/cp-redis.gpg] https://packages.redis.io/deb %s main", code)); err != nil {
			return err
		}
	}
	if err := aptUpdate(); err != nil {
		return err
	}
	return aptInstall("redis-server")
}

func buildRedis(version string) error {
	candidates := redisReleaseTags(version)
	tmp := "/var/lib/siroc/build/redis"
	_ = os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0755); err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	var last error
	for _, tag := range candidates {
		tgz := filepath.Join(tmp, "redis.tgz")
		url := "https://download.redis.io/releases/redis-" + tag + ".tar.gz"
		dl := exec.Command("curl", "-fsSL", "-o", tgz, url)
		if out, err := combinedTimeout(dl, 2*time.Minute); err != nil {
			last = fmt.Errorf("download %s: %s", tag, tail(out))
			continue
		}
		if out, err := combinedTimeout(exec.Command("tar", "-xzf", tgz, "-C", tmp), 1*time.Minute); err != nil {
			last = fmt.Errorf("extract %s: %s", tag, tail(out))
			continue
		}
		src := filepath.Join(tmp, "redis-"+tag)
		deps := exec.Command("make", "-C", "deps")
		deps.Dir = src
		_ = combinedTimeoutWait(deps, 4*time.Minute)
		if err := makeRedis(src, []string{"-j2", "BUILD_TLS=yes"}); err != nil {
			clean := exec.Command("make", "clean")
			clean.Dir = src
			_, _ = combinedTimeout(clean, 1*time.Minute)
			if err2 := makeRedis(src, []string{"-j2", "BUILD_TLS=yes", "MALLOC=libc"}); err2 != nil {
				last = fmt.Errorf("compile %s: %v", tag, err2)
				continue
			}
		}
		inst := exec.Command("make", "PREFIX=/usr/local", "install")
		inst.Dir = src
		if out, err := combinedTimeout(inst, 2*time.Minute); err != nil {
			last = fmt.Errorf("install %s: %s", tag, tail(out))
			continue
		}
		srcConf := filepath.Join(src, "redis.conf")
		if _, err := os.Stat(redisConfPath); err != nil {
			_ = os.MkdirAll("/etc/redis", 0755)
			_ = exec.Command("cp", srcConf, redisConfPath).Run()
		}
		return nil
	}
	if last == nil {
		last = fmt.Errorf("no redis tarball for %s", version)
	}
	return last
}

func makeRedis(src string, args []string) error {
	cmd := exec.Command("make", args...)
	cmd.Dir = src
	out, err := combinedTimeout(cmd, 8*time.Minute)
	if err != nil {
		return fmt.Errorf("%s", tail(out))
	}
	return nil
}

func combinedTimeoutWait(cmd *exec.Cmd, d time.Duration) string {
	out, _ := combinedTimeout(cmd, d)
	return out
}

func redisReleaseTags(ver string) []string {
	switch ver {
	case "7.0":
		return []string{"7.0.15", "7.0.14", "7.0.13"}
	case "7.2":
		return []string{"7.2.11", "7.2.10", "7.2.9"}
	case "7.4":
		return []string{"7.4.5", "7.4.3", "7.4.2"}
	case "8.0":
		return []string{"8.0.4", "8.0.3", "8.0.2"}
	case "8.2":
		return []string{"8.2.2", "8.2.1", "8.2.0"}
	default:
		return []string{ver + ".0"}
	}
}

func writeRedisUnit() error {
	bin := "/usr/local/bin/redis-server"
	if _, err := os.Stat(bin); err != nil {
		if p, look := exec.LookPath("redis-server"); look == nil {
			bin = p
		} else {
			bin = "/usr/bin/redis-server"
		}
	}
	dropDir := "/etc/systemd/system/redis-server.service.d"
	if err := os.MkdirAll(dropDir, 0755); err != nil {
		return err
	}
	drop := `[Service]
Type=simple
ExecStart=
ExecStart=` + bin + ` ` + redisConfPath + `
`
	if err := os.WriteFile(filepath.Join(dropDir, "cp.conf"), []byte(drop), 0644); err != nil {
		return err
	}
	path := "/etc/systemd/system/redis-server.service"
	if _, err := os.Stat("/lib/systemd/system/redis-server.service"); err == nil {
		return nil
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	unit := `[Unit]
Description=Redis In-Memory Data Store
After=network.target

[Service]
Type=simple
User=redis
Group=redis
ExecStart=` + bin + ` ` + redisConfPath + `
Restart=on-failure
LimitNOFILE=65535
RuntimeDirectory=redis
RuntimeDirectoryMode=0755

[Install]
WantedBy=multi-user.target
`
	return os.WriteFile(path, []byte(unit), 0644)
}

func defaultRedisConf() string {
	return `bind 127.0.0.1 -::1
protected-mode yes
port 6379
tcp-backlog 511
timeout 0
daemonize no
supervised no
pidfile /run/redis/redis-server.pid
loglevel notice
logfile /var/log/redis/redis-server.log
databases 16
dir /var/lib/redis
appendonly no
maxmemory 0
maxmemory-policy noeviction
`
}

func GetRedis() (*rpc.RedisSettings, error) {
	st := &rpc.RedisSettings{
		Bind:            "127.0.0.1",
		Port:            6379,
		ProtectedMode:   true,
		MaxMemoryPolicy: "noeviction",
		Databases:       16,
		MaxMemory:       "0",
	}
	ok, ver := redisInstalled()
	st.Installed = ok
	st.Version = ver
	if !ok {
		return st, nil
	}
	if b, err := os.ReadFile(redisStatePath); err == nil {
		_ = json.Unmarshal(b, st)
		st.Installed = true
		st.Version = ver
	}
	if b, err := os.ReadFile(redisConfPath); err == nil {
		mergeRedisConf(st, string(b))
	}
	st.HasPassword = redisHasPassword()
	st.Password = ""
	return st, nil
}

func redisHasPassword() bool {
	b, err := os.ReadFile(redisConfPath)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "requirepass ") && !strings.HasPrefix(t, "requirepass \"\"") {
			return true
		}
	}
	return false
}

func mergeRedisConf(st *rpc.RedisSettings, conf string) {
	for _, line := range strings.Split(conf, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		key, val, ok := strings.Cut(t, " ")
		if !ok {
			continue
		}
		val = strings.TrimSpace(val)
		switch key {
		case "bind":
			st.Bind = val
		case "port":
			if n, err := strconv.Atoi(val); err == nil {
				st.Port = n
			}
		case "protected-mode":
			st.ProtectedMode = val == "yes"
		case "maxmemory":
			st.MaxMemory = val
		case "maxmemory-policy":
			st.MaxMemoryPolicy = val
		case "appendonly":
			st.AppendOnly = val == "yes"
		case "timeout":
			if n, err := strconv.Atoi(val); err == nil {
				st.Timeout = n
			}
		case "databases":
			if n, err := strconv.Atoi(val); err == nil {
				st.Databases = n
			}
		}
	}
}

func ApplyRedis(in rpc.RedisSettings) error {
	if !fileOKRedis(redisConfPath) {
		if err := os.MkdirAll("/etc/redis", 0755); err != nil {
			return err
		}
		if err := os.WriteFile(redisConfPath, []byte(defaultRedisConf()), 0640); err != nil {
			return err
		}
	}
	cur, _ := GetRedis()
	if cur == nil {
		cur = &rpc.RedisSettings{}
	}
	bind := strings.TrimSpace(in.Bind)
	if bind == "" {
		bind = "127.0.0.1"
	}
	port := in.Port
	if port < 1 || port > 65535 {
		port = 6379
	}
	policy := strings.TrimSpace(in.MaxMemoryPolicy)
	switch policy {
	case "noeviction", "allkeys-lru", "allkeys-lfu", "volatile-lru", "volatile-lfu", "allkeys-random", "volatile-random", "volatile-ttl":
	default:
		policy = "noeviction"
	}
	mem := strings.TrimSpace(in.MaxMemory)
	if mem == "" {
		mem = "0"
	}
	if in.Databases < 1 || in.Databases > 1024 {
		in.Databases = 16
	}
	if in.Timeout < 0 {
		in.Timeout = 0
	}
	if bind != "127.0.0.1" && bind != "127.0.0.1 -::1" && strings.TrimSpace(in.Password) == "" && !in.ClearPassword && !cur.HasPassword {
		return fmt.Errorf("set a password before binding Redis on a public address")
	}

	b, err := os.ReadFile(redisConfPath)
	if err != nil {
		return err
	}
	conf := string(b)
	conf = setRedisLine(conf, "bind", bind)
	conf = setRedisLine(conf, "port", strconv.Itoa(port))
	if in.ProtectedMode {
		conf = setRedisLine(conf, "protected-mode", "yes")
	} else {
		conf = setRedisLine(conf, "protected-mode", "no")
	}
	conf = setRedisLine(conf, "maxmemory", mem)
	conf = setRedisLine(conf, "maxmemory-policy", policy)
	if in.AppendOnly {
		conf = setRedisLine(conf, "appendonly", "yes")
	} else {
		conf = setRedisLine(conf, "appendonly", "no")
	}
	conf = setRedisLine(conf, "timeout", strconv.Itoa(in.Timeout))
	conf = setRedisLine(conf, "databases", strconv.Itoa(in.Databases))
	if in.ClearPassword {
		conf = commentRedisLine(conf, "requirepass")
	} else if strings.TrimSpace(in.Password) != "" {
		if strings.ContainsAny(in.Password, " \t\"'") {
			return fmt.Errorf("password must not contain spaces or quotes")
		}
		conf = setRedisLine(conf, "requirepass", in.Password)
	}
	if err := os.WriteFile(redisConfPath, []byte(conf), 0640); err != nil {
		return err
	}
	_ = exec.Command("chown", "redis:redis", redisConfPath).Run()
	save := rpc.RedisSettings{
		Bind:            bind,
		Port:            port,
		ProtectedMode:   in.ProtectedMode,
		MaxMemory:       mem,
		MaxMemoryPolicy: policy,
		AppendOnly:      in.AppendOnly,
		Timeout:         in.Timeout,
		Databases:       in.Databases,
		HasPassword:     !in.ClearPassword && (strings.TrimSpace(in.Password) != "" || cur.HasPassword),
	}
	raw, _ := json.Marshal(save)
	_ = os.MkdirAll(filepath.Dir(redisStatePath), 0750)
	_ = os.WriteFile(redisStatePath, raw, 0640)
	out, err := exec.Command("systemctl", "restart", "redis-server").CombinedOutput()
	if err != nil {
		return fmt.Errorf("restart redis: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func fileOKRedis(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Size() > 0
}

func setRedisLine(conf, key, val string) string {
	lines := strings.Split(conf, "\n")
	found := false
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "#") {
			continue
		}
		if t == key || strings.HasPrefix(t, key+" ") {
			if !found {
				lines[i] = key + " " + val
				found = true
			} else {
				lines[i] = "# " + t
			}
		}
	}
	if !found {
		if !strings.HasSuffix(conf, "\n") && conf != "" {
			conf += "\n"
		}
		return conf + key + " " + val + "\n"
	}
	return strings.Join(lines, "\n")
}

func commentRedisLine(conf, key string) string {
	lines := strings.Split(conf, "\n")
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, key+" ") {
			lines[i] = "# " + t
		}
	}
	return strings.Join(lines, "\n")
}

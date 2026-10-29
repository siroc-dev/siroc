//go:build linux

package software

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/validate"
)

// Per-account Redis runs a dedicated redis-server instance for each user so the
// memory cap is a real, kernel-enforced limit instead of a shared ACL on one
// server. Each instance listens on its own loopback TCP port with a random
// password, has persistence disabled (it is a cache), and evicts the account's
// own keys with allkeys-lru when it reaches maxmemory.
const (
	redisUserStateDir     = "/var/lib/siroc/redis-users"
	redisInstConfDir      = "/etc/redis"
	redisInstDataBase     = "/var/lib/redis-siroc"
	redisInstRunDir       = "/run/redis-siroc"
	redisInstUnitPath     = "/etc/systemd/system/redis-siroc@.service"
	redisUserPortMin      = 6400
	redisUserPortMax      = 6999
	defaultUserRedisMaxMB = 2048
	minUserRedisMaxMB     = 16
	maxUserRedisMaxMB     = 65536
)

func (m *Manager) UserRedis(username string) (*rpc.UserRedis, error) {
	return getUserRedis(username)
}

func (m *Manager) ListUserRedis() ([]rpc.UserRedis, error) {
	return listUserRedis()
}

func (m *Manager) SetUserRedis(in rpc.UserRedisReq) (*rpc.UserRedis, error) {
	return setUserRedis(in)
}

func (m *Manager) DropUserRedis(username string) {
	_ = disableUserRedis(username)
}

func redisUserStatePath(username string) string {
	return filepath.Join(redisUserStateDir, username+".json")
}

func redisInstConfPath(username string) string {
	return filepath.Join(redisInstConfDir, "siroc-"+username+".conf")
}

func redisInstDataDir(username string) string {
	return filepath.Join(redisInstDataBase, username)
}

func redisInstService(username string) string {
	return "redis-siroc@" + username
}

func getUserRedis(username string) (*rpc.UserRedis, error) {
	if err := validate.LinuxUser(username); err != nil {
		return nil, err
	}
	ok, _ := redisInstalled()
	st := &rpc.UserRedis{
		Username:    username,
		Installed:   ok,
		Host:        "127.0.0.1",
		Database:    0,
		MaxMemoryMB: defaultUserRedisMaxMB,
	}
	if saved := readUserRedisState(username); saved != nil {
		st.Enabled = saved.Enabled
		st.Password = saved.Password
		st.Port = saved.Port
		if saved.MaxMemoryMB > 0 {
			st.MaxMemoryMB = saved.MaxMemoryMB
		}
		st.File = saved.File
	}
	if !ok {
		st.Message = "Install Redis from Software first."
		return st, nil
	}
	if st.Enabled && st.Port > 0 && redisInstActive(username) {
		st.Active = true
		if used, err := redisInstUsedMB(st.Port, st.Password); err == nil {
			st.UsedMemoryMB = used
		}
	}
	if st.File == "" {
		if u, err := user.Lookup(username); err == nil {
			st.File = filepath.Join(u.HomeDir, ".cp", "redis.json")
		}
	}
	return st, nil
}

func listUserRedis() ([]rpc.UserRedis, error) {
	entries, err := os.ReadDir(redisUserStateDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []rpc.UserRedis{}, nil
		}
		return nil, err
	}
	var out []rpc.UserRedis
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		if validate.LinuxUser(name) != nil {
			continue
		}
		st, err := getUserRedis(name)
		if err != nil || st == nil {
			continue
		}
		st.Password = ""
		out = append(out, *st)
	}
	return out, nil
}

func setUserRedis(in rpc.UserRedisReq) (*rpc.UserRedis, error) {
	if err := validate.LinuxUser(in.Username); err != nil {
		return nil, err
	}
	if _, err := user.Lookup(in.Username); err != nil {
		return nil, fmt.Errorf("user not found")
	}
	ok, _ := redisInstalled()
	if !ok {
		return nil, fmt.Errorf("install Redis from Software first")
	}
	if in.MaxMemoryMB != 0 && (in.MaxMemoryMB < minUserRedisMaxMB || in.MaxMemoryMB > maxUserRedisMaxMB) {
		return nil, fmt.Errorf("memory limit must be between %d and %d MB", minUserRedisMaxMB, maxUserRedisMaxMB)
	}
	if in.Enabled != nil && !*in.Enabled {
		if err := disableUserRedis(in.Username); err != nil {
			return nil, err
		}
		return getUserRedis(in.Username)
	}
	saved := readUserRedisState(in.Username)
	maxMB := in.MaxMemoryMB
	if maxMB <= 0 {
		if saved != nil && saved.MaxMemoryMB > 0 {
			maxMB = saved.MaxMemoryMB
		} else {
			maxMB = defaultUserRedisMaxMB
		}
	}
	rotate := in.RotatePassword || (in.Enabled != nil && *in.Enabled)
	if saved != nil && saved.Enabled && saved.Password != "" && !in.RotatePassword && (in.Enabled == nil || *in.Enabled) {
		rotate = false
	}
	if err := enableUserRedis(in.Username, maxMB, rotate); err != nil {
		return nil, err
	}
	st, err := getUserRedis(in.Username)
	if err != nil {
		return nil, err
	}
	switch {
	case rotate:
		st.Message = "Redis password was generated"
	case in.MaxMemoryMB > 0:
		st.Message = fmt.Sprintf("Memory limit set to %d MB", maxMB)
	default:
		st.Message = "Redis is enabled"
	}
	return st, nil
}

func enableUserRedis(username string, maxMB int64, rotate bool) error {
	if err := installUserRedisUnit(); err != nil {
		return err
	}
	saved := readUserRedisState(username)
	port := 0
	oldPass := ""
	if saved != nil {
		port = saved.Port
		oldPass = saved.Password
	}
	if port == 0 {
		p, err := pickFreeRedisPort()
		if err != nil {
			return err
		}
		port = p
	}
	pass := oldPass
	if rotate || pass == "" {
		p, err := randomRedisPass()
		if err != nil {
			return err
		}
		pass = p
	}
	if maxMB <= 0 {
		maxMB = defaultUserRedisMaxMB
	}
	_ = os.MkdirAll(redisInstRunDir, 0755)
	if err := writeUserRedisConf(username, port, pass, maxMB); err != nil {
		return err
	}
	_ = exec.Command("systemctl", "enable", redisInstService(username)).Run()
	if redisInstActive(username) {
		// Apply live so the cache is not dropped on a limit or password change.
		authPass := oldPass
		if authPass == "" {
			authPass = pass
		}
		bytes := strconv.FormatInt(maxMB*1024*1024, 10)
		if _, err := redisCLIPort(port, authPass, "CONFIG", "SET", "maxmemory", bytes); err != nil {
			return fmt.Errorf("apply memory limit: %w", err)
		}
		_, _ = redisCLIPort(port, authPass, "CONFIG", "SET", "maxmemory-policy", "allkeys-lru")
		if rotate && authPass != "" {
			_, _ = redisCLIPort(port, authPass, "CONFIG", "SET", "requirepass", pass)
		}
	} else {
		out, err := exec.Command("systemctl", "restart", redisInstService(username)).CombinedOutput()
		if err != nil {
			return fmt.Errorf("start redis: %s", strings.TrimSpace(string(out)))
		}
		if err := waitRedisInstReady(port, pass, 10*time.Second); err != nil {
			return fmt.Errorf("redis is not responding: %w", err)
		}
	}
	u, err := user.Lookup(username)
	if err != nil {
		return fmt.Errorf("user not found")
	}
	st := rpc.UserRedis{
		Username:    username,
		Enabled:     true,
		Installed:   true,
		Host:        "127.0.0.1",
		Port:        port,
		Password:    pass,
		Database:    0,
		MaxMemoryMB: maxMB,
		File:        filepath.Join(u.HomeDir, ".cp", "redis.json"),
	}
	if err := writeUserRedisState(&st); err != nil {
		return err
	}
	return writeUserRedisHome(&st)
}

func disableUserRedis(username string) error {
	if validate.LinuxUser(username) != nil {
		return nil
	}
	_ = exec.Command("systemctl", "disable", "--now", redisInstService(username)).Run()
	_ = os.Remove(redisInstConfPath(username))
	_ = os.RemoveAll(redisInstDataDir(username))
	_ = os.Remove(filepath.Join(redisInstRunDir, username+".pid"))
	if u, err := user.Lookup(username); err == nil {
		_ = os.Remove(filepath.Join(u.HomeDir, ".cp", "redis.json"))
		_ = os.Remove(filepath.Join(u.HomeDir, ".cp", "redis.env"))
	}
	_ = os.Remove(redisUserStatePath(username))
	return nil
}

func writeUserRedisConf(username string, port int, pass string, maxMB int64) error {
	// The base dir must be owned by and traversable for the redis user, or
	// redis-server cannot chdir into its per-account subdir ("dir" directive)
	// and exits at config-load time with a permission error.
	if err := os.MkdirAll(redisInstDataBase, 0755); err != nil {
		return err
	}
	_ = exec.Command("chown", "redis:redis", redisInstDataBase).Run()
	dataDir := redisInstDataDir(username)
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return err
	}
	_ = exec.Command("chown", "-R", "redis:redis", dataDir).Run()
	bytes := maxMB * 1024 * 1024
	conf := strings.Join([]string{
		"port " + strconv.Itoa(port),
		"bind 127.0.0.1 -::1",
		"protected-mode yes",
		"daemonize no",
		"supervised systemd",
		"pidfile " + filepath.Join(redisInstRunDir, username+".pid"),
		"dir " + dataDir,
		"maxmemory " + strconv.FormatInt(bytes, 10),
		"maxmemory-policy allkeys-lru",
		`save ""`,
		"appendonly no",
		"requirepass " + pass,
		"loglevel notice",
		"",
	}, "\n")
	path := redisInstConfPath(username)
	if err := os.WriteFile(path, []byte(conf), 0640); err != nil {
		return err
	}
	_ = exec.Command("chown", "redis:redis", path).Run()
	return nil
}

func installUserRedisUnit() error {
	bin := redisServerBin()
	unit := `[Unit]
Description=Siroc per-account Redis (%i)
After=network.target

[Service]
Type=simple
User=redis
Group=redis
RuntimeDirectory=redis-siroc
RuntimeDirectoryMode=0755
ExecStart=` + bin + ` /etc/redis/siroc-%i.conf --supervised systemd
Restart=on-failure
RestartSec=2
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
`
	if old, err := os.ReadFile(redisInstUnitPath); err == nil && string(old) == unit {
		return nil
	}
	if err := os.WriteFile(redisInstUnitPath, []byte(unit), 0644); err != nil {
		return err
	}
	return exec.Command("systemctl", "daemon-reload").Run()
}

func redisServerBin() string {
	bin := "/usr/local/bin/redis-server"
	if _, err := os.Stat(bin); err != nil {
		if p, look := exec.LookPath("redis-server"); look == nil {
			bin = p
		} else {
			bin = "/usr/bin/redis-server"
		}
	}
	return bin
}

func redisInstActive(username string) bool {
	return exec.Command("systemctl", "is-active", "--quiet", redisInstService(username)).Run() == nil
}

func usedRedisPorts() map[int]bool {
	used := map[int]bool{}
	entries, err := os.ReadDir(redisUserStateDir)
	if err != nil {
		return used
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(redisUserStateDir, e.Name()))
		if err != nil {
			continue
		}
		var st rpc.UserRedis
		if json.Unmarshal(b, &st) == nil && st.Port > 0 {
			used[st.Port] = true
		}
	}
	return used
}

func pickFreeRedisPort() (int, error) {
	used := usedRedisPorts()
	// Never hand out the shared system Redis port; it stays reserved for the
	// main instance even if the per-account range is ever widened.
	used[redisPort()] = true
	used[6379] = true
	span := redisUserPortMax - redisUserPortMin + 1
	for i := 0; i < 400; i++ {
		p := redisUserPortMin + randInt(span)
		if used[p] {
			continue
		}
		if portFree(p) {
			return p, nil
		}
	}
	return 0, fmt.Errorf("no free redis port available in %d-%d", redisUserPortMin, redisUserPortMax)
}

func portFree(port int) bool {
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func randInt(n int) int {
	if n <= 0 {
		return 0
	}
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0
	}
	return int(binary.BigEndian.Uint32(b[:]) % uint32(n))
}

func waitRedisInstReady(port int, pass string, d time.Duration) error {
	deadline := time.Now().Add(d)
	var last error
	for time.Now().Before(deadline) {
		if _, err := redisCLIPort(port, pass, "PING"); err == nil {
			return nil
		} else {
			last = err
		}
		time.Sleep(250 * time.Millisecond)
	}
	if last == nil {
		last = fmt.Errorf("redis did not become ready")
	}
	return last
}

func redisInstUsedMB(port int, pass string) (int64, error) {
	out, err := redisCLIPort(port, pass, "INFO", "memory")
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "used_memory:") {
			v, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "used_memory:")), 10, 64)
			if err != nil {
				return 0, err
			}
			return v / (1024 * 1024), nil
		}
	}
	return 0, fmt.Errorf("used_memory not found")
}

func writeUserRedisState(st *rpc.UserRedis) error {
	if err := os.MkdirAll(redisUserStateDir, 0750); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(redisUserStatePath(st.Username), raw, 0640)
}

func readUserRedisState(username string) *rpc.UserRedis {
	b, err := os.ReadFile(redisUserStatePath(username))
	if err != nil {
		return nil
	}
	var st rpc.UserRedis
	if json.Unmarshal(b, &st) != nil {
		return nil
	}
	return &st
}

func writeUserRedisHome(st *rpc.UserRedis) error {
	u, err := user.Lookup(st.Username)
	if err != nil {
		return err
	}
	dir := filepath.Join(u.HomeDir, ".cp")
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(map[string]any{
		"host":     st.Host,
		"port":     st.Port,
		"username": st.RedisUser,
		"password": st.Password,
		"database": st.Database,
	}, "", "  ")
	if err != nil {
		return err
	}
	jsonPath := filepath.Join(dir, "redis.json")
	envPath := filepath.Join(dir, "redis.env")
	env := fmt.Sprintf("REDIS_HOST=%s\nREDIS_PORT=%d\nREDIS_USERNAME=%s\nREDIS_PASSWORD=%s\nREDIS_DATABASE=%d\n",
		st.Host, st.Port, st.RedisUser, st.Password, st.Database)
	if err := os.WriteFile(jsonPath, raw, 0640); err != nil {
		return err
	}
	if err := os.WriteFile(envPath, []byte(env), 0640); err != nil {
		return err
	}
	own := st.Username + ":" + st.Username
	_ = exec.Command("chown", "-R", own, dir).Run()
	_ = os.Chmod(dir, 0750)
	_ = os.Chmod(jsonPath, 0640)
	_ = os.Chmod(envPath, 0640)
	return nil
}

func randomRedisPass() (string, error) {
	const chars = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, 20)
	for i, v := range b {
		out[i] = chars[int(v)%len(chars)]
	}
	return string(out), nil
}

func redisRequirePass() string {
	b, err := os.ReadFile(redisConfPath)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "requirepass ") {
			return strings.TrimSpace(strings.TrimPrefix(t, "requirepass "))
		}
	}
	return ""
}

func redisPort() int {
	if rs, err := GetRedis(); err == nil && rs != nil && rs.Port > 0 {
		return rs.Port
	}
	return 6379
}

// redisCLI talks to the shared system Redis (used by global settings and status).
func redisCLI(args ...string) (string, error) {
	out, err := redisCLIAuth(true, args...)
	if err != nil && redisRequirePass() != "" && redisAuthFailed(out+" "+err.Error()) {
		return redisCLIAuth(false, args...)
	}
	return out, err
}

func redisAuthFailed(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "noauth") || strings.Contains(m, "wrongpass") || strings.Contains(m, "invalid password") || strings.Contains(m, "auth")
}

func redisCLIAuth(usePass bool, args ...string) (string, error) {
	bin, err := exec.LookPath("redis-cli")
	if err != nil {
		return "", fmt.Errorf("redis-cli not found")
	}
	cmdArgs := append([]string{"-h", "127.0.0.1", "-p", strconv.Itoa(redisPort()), "--no-auth-warning"}, args...)
	cmd := exec.Command(bin, cmdArgs...)
	if usePass {
		if pass := redisRequirePass(); pass != "" {
			cmd.Env = append(os.Environ(), "REDISCLI_AUTH="+pass)
		}
	}
	out, err := combinedTimeout(cmd, 15*time.Second)
	msg := strings.TrimSpace(out)
	if err != nil {
		if msg == "" {
			msg = err.Error()
		}
		return msg, fmt.Errorf("%s", tail(msg))
	}
	if strings.HasPrefix(strings.ToUpper(msg), "ERR ") {
		return msg, fmt.Errorf("%s", msg)
	}
	return msg, nil
}

// redisCLIPort talks to one account's dedicated Redis instance on its own port.
func redisCLIPort(port int, pass string, args ...string) (string, error) {
	bin, err := exec.LookPath("redis-cli")
	if err != nil {
		return "", fmt.Errorf("redis-cli not found")
	}
	cmdArgs := append([]string{"-h", "127.0.0.1", "-p", strconv.Itoa(port), "--no-auth-warning"}, args...)
	cmd := exec.Command(bin, cmdArgs...)
	if pass != "" {
		cmd.Env = append(os.Environ(), "REDISCLI_AUTH="+pass)
	}
	out, err := combinedTimeout(cmd, 15*time.Second)
	msg := strings.TrimSpace(out)
	if err != nil {
		if msg == "" {
			msg = err.Error()
		}
		return msg, fmt.Errorf("%s", tail(msg))
	}
	if strings.HasPrefix(strings.ToUpper(msg), "ERR ") {
		return msg, fmt.Errorf("%s", msg)
	}
	return msg, nil
}

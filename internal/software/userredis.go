//go:build linux

package software

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
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

const (
	redisACLPath      = "/etc/redis/users.acl"
	redisUserStateDir = "/var/lib/siroc/redis-users"
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

func redisUserPrefix(username string) string {
	return username + ":"
}

func getUserRedis(username string) (*rpc.UserRedis, error) {
	if err := validate.LinuxUser(username); err != nil {
		return nil, err
	}
	ok, _ := redisInstalled()
	st := &rpc.UserRedis{
		Username:  username,
		Installed: ok,
		Host:      "127.0.0.1",
		Port:      6379,
		RedisUser: username,
		Prefix:    redisUserPrefix(username),
		Database:  0,
	}
	if rs, err := GetRedis(); err == nil && rs != nil {
		st.Host = firstRedisBind(rs.Bind)
		if rs.Port > 0 {
			st.Port = rs.Port
		}
	}
	if saved := readUserRedisState(username); saved != nil {
		st.Enabled = saved.Enabled
		st.Password = saved.Password
		if saved.Host != "" {
			st.Host = saved.Host
		}
		if saved.Port > 0 {
			st.Port = saved.Port
		}
		if saved.Prefix != "" {
			st.Prefix = saved.Prefix
		}
		st.Database = saved.Database
		st.File = saved.File
	}
	if !ok {
		st.Message = "Install Redis from Software first."
		return st, nil
	}
	if live, _ := redisACLHasUser(username); live {
		st.Enabled = true
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
	if in.Enabled != nil && !*in.Enabled {
		if err := disableUserRedis(in.Username); err != nil {
			return nil, err
		}
		return getUserRedis(in.Username)
	}
	rotate := in.RotatePassword || in.Enabled != nil && *in.Enabled
	cur := readUserRedisState(in.Username)
	if cur != nil && cur.Enabled && cur.Password != "" && !in.RotatePassword && (in.Enabled == nil || *in.Enabled) {
		rotate = false
	}
	if err := enableUserRedis(in.Username, rotate); err != nil {
		return nil, err
	}
	st, err := getUserRedis(in.Username)
	if err != nil {
		return nil, err
	}
	if rotate {
		st.Message = "Redis password was generated"
	} else {
		st.Message = "Redis is enabled"
	}
	return st, nil
}

func enableUserRedis(username string, rotate bool) error {
	if err := ensureRedisACLFile(); err != nil {
		return err
	}
	pass := ""
	if saved := readUserRedisState(username); saved != nil {
		pass = saved.Password
	}
	if rotate || pass == "" {
		p, err := randomRedisPass()
		if err != nil {
			return err
		}
		pass = p
	}
	rs, _ := GetRedis()
	host := "127.0.0.1"
	port := 6379
	if rs != nil {
		host = firstRedisBind(rs.Bind)
		if rs.Port > 0 {
			port = rs.Port
		}
	}
	prefix := redisUserPrefix(username)
	args := []string{
		"ACL", "SETUSER", username, "reset", "on", ">" + pass,
		"~" + prefix + "*", "&*",
		"-@all", "+@read", "+@write", "+@string", "+@hash", "+@list", "+@set", "+@sortedset", "+@stream", "+@bitmap", "+@geo",
		"-@dangerous", "-@admin", "+ping", "+echo", "+select", "+info",
	}
	if out, err := redisCLI(args...); err != nil {
		return fmt.Errorf("redis ACL: %s", strings.TrimSpace(out+" "+err.Error()))
	}
	if out, err := redisCLI("ACL", "SAVE"); err != nil {
		return fmt.Errorf("redis ACL SAVE: %s", strings.TrimSpace(out+" "+err.Error()))
	}
	u, err := user.Lookup(username)
	if err != nil {
		return fmt.Errorf("user not found")
	}
	file := filepath.Join(u.HomeDir, ".cp", "redis.json")
	st := rpc.UserRedis{
		Username:  username,
		Enabled:   true,
		Installed: true,
		Host:      host,
		Port:      port,
		RedisUser: username,
		Password:  pass,
		Prefix:    prefix,
		Database:  0,
		File:      file,
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
	ok, _ := redisInstalled()
	if ok {
		_, _ = redisCLI("ACL", "DELUSER", username)
		_, _ = redisCLI("ACL", "SAVE")
	}
	if u, err := user.Lookup(username); err == nil {
		_ = os.Remove(filepath.Join(u.HomeDir, ".cp", "redis.json"))
		_ = os.Remove(filepath.Join(u.HomeDir, ".cp", "redis.env"))
	}
	_ = os.Remove(redisUserStatePath(username))
	return nil
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
		"prefix":   st.Prefix,
		"database": st.Database,
	}, "", "  ")
	if err != nil {
		return err
	}
	jsonPath := filepath.Join(dir, "redis.json")
	envPath := filepath.Join(dir, "redis.env")
	env := fmt.Sprintf("REDIS_HOST=%s\nREDIS_PORT=%d\nREDIS_USERNAME=%s\nREDIS_PASSWORD=%s\nREDIS_PREFIX=%s\nREDIS_DATABASE=%d\n",
		st.Host, st.Port, st.RedisUser, st.Password, st.Prefix, st.Database)
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

func firstRedisBind(bind string) string {
	bind = strings.TrimSpace(bind)
	if bind == "" {
		return "127.0.0.1"
	}
	fields := strings.Fields(bind)
	if len(fields) == 0 {
		return "127.0.0.1"
	}
	h := fields[0]
	if h == "0.0.0.0" || h == "*" || h == "-::1" || h == "::1" {
		return "127.0.0.1"
	}
	return h
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

func redisACLHasUser(username string) (bool, error) {
	out, err := redisCLI("ACL", "GETUSER", username)
	if err != nil {
		if strings.Contains(strings.ToLower(out+" "+err.Error()), "unknown command") {
			return false, fmt.Errorf("Redis 6+ is required for per-user ACL")
		}
		return false, err
	}
	t := strings.TrimSpace(out)
	return t != "" && t != "(nil)" && !strings.EqualFold(t, "empty array") && t != "null", nil
}

func redisConfHasKey(conf, key string) bool {
	prefix := key + " "
	for _, line := range strings.Split(conf, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if t == key || strings.HasPrefix(t, prefix) {
			return true
		}
	}
	return false
}

func waitRedisReady(d time.Duration) error {
	deadline := time.Now().Add(d)
	var last error
	for time.Now().Before(deadline) {
		if _, err := redisCLI("PING"); err == nil {
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

func ensureRedisACLFile() error {
	if !serviceActive("redis-server") {
		_ = exec.Command("systemctl", "start", "redis-server").Run()
		if err := waitRedisReady(8 * time.Second); err != nil {
			return err
		}
	}
	if err := os.MkdirAll("/etc/redis", 0755); err != nil {
		return err
	}
	b, err := os.ReadFile(redisConfPath)
	if err != nil {
		return fmt.Errorf("redis.conf not found")
	}
	needRestart := false
	if !redisConfHasKey(string(b), "aclfile") {
		conf := setRedisLine(string(b), "aclfile", redisACLPath)
		if err := os.WriteFile(redisConfPath, []byte(conf), 0640); err != nil {
			return err
		}
		_ = exec.Command("chown", "redis:redis", redisConfPath).Run()
		needRestart = true
	}
	if _, err := os.Stat(redisACLPath); err != nil {
		body := "user default on nopass ~* &* +@all\n"
		if list, err := redisCLI("ACL", "LIST"); err == nil && strings.TrimSpace(list) != "" {
			body = strings.TrimSpace(list) + "\n"
		} else if pass := redisRequirePass(); pass != "" {
			body = "user default on >" + pass + " ~* &* +@all\n"
		}
		if err := os.WriteFile(redisACLPath, []byte(body), 0640); err != nil {
			return err
		}
		_ = exec.Command("chown", "redis:redis", redisACLPath).Run()
		needRestart = true
	}
	if needRestart {
		out, err := exec.Command("systemctl", "restart", "redis-server").CombinedOutput()
		if err != nil {
			return fmt.Errorf("restart redis: %s", strings.TrimSpace(string(out)))
		}
	}
	if err := waitRedisReady(10 * time.Second); err != nil {
		return fmt.Errorf("redis is not responding: %w", err)
	}
	if _, err := redisCLI("ACL", "LIST"); err != nil {
		return fmt.Errorf("Redis 6+ ACL is required: %w", err)
	}
	return nil
}

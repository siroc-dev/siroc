//go:build linux

package security

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/siroc-dev/siroc/internal/auth"
)

const (
	fail2banFilter      = "/etc/fail2ban/filter.d/siroc.conf"
	fail2banJail        = "/etc/fail2ban/jail.d/siroc.conf"
	fail2banIgnoreStore = "/var/lib/siroc/fail2ban-ignore"
	fail2banIgnoreJail  = "/etc/fail2ban/jail.d/siroc-ignore.local"
)

func EnsureFail2ban() error {
	if _, err := exec.LookPath("fail2ban-client"); err != nil {
		return nil
	}
	if err := os.MkdirAll("/var/log/siroc", 0750); err != nil {
		return err
	}
	if u, err := user.Lookup("siroc"); err == nil {
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		_ = os.Chown("/var/log/siroc", uid, gid)
		if st, err := os.Stat(auth.AuthLogPath); err == nil && !st.IsDir() {
			_ = os.Chown(auth.AuthLogPath, uid, gid)
		}
	}
	if _, err := os.Stat(auth.AuthLogPath); err != nil {
		f, err := os.OpenFile(auth.AuthLogPath, os.O_CREATE|os.O_WRONLY, 0640)
		if err == nil {
			_ = f.Close()
			if u, err := user.Lookup("siroc"); err == nil {
				uid, _ := strconv.Atoi(u.Uid)
				gid, _ := strconv.Atoi(u.Gid)
				_ = os.Chown(auth.AuthLogPath, uid, gid)
			}
		}
	}
	filter := `[Definition]
failregex = siroc login failed ip=<HOST>
ignoreregex =
datepattern = ^%%Y-%%m-%%d %%H:%%M:%%S
`
	if err := os.WriteFile(fail2banFilter, []byte(filter), 0644); err != nil {
		return err
	}
	writeExtraFilters()
	ensureServiceLogs()
	port := CPWebPort()
	jail := RenderFail2banJails(fail2banJailSpecs(port))
	if err := os.WriteFile(fail2banJail, []byte(jail), 0644); err != nil {
		return err
	}
	if _, err := os.Stat("/etc/fail2ban/jail.local"); err != nil {
		_ = os.WriteFile("/etc/fail2ban/jail.local", []byte("[DEFAULT]\nbantime = 1h\nfindtime = 10m\nmaxretry = 5\n\n[sshd]\nenabled = true\n"), 0644)
	}
	if err := writeFail2banIgnoreJail(); err != nil {
		return err
	}
	_ = exec.Command("systemctl", "enable", "--now", "fail2ban").Run()
	if exec.Command("systemctl", "is-active", "--quiet", "fail2ban").Run() == nil {
		_ = exec.Command("fail2ban-client", "reload").Run()
	}
	return nil
}

func ReadFail2banIgnore() ([]string, error) {
	b, err := os.ReadFile(fail2banIgnoreStore)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	return NormalizeIgnore(strings.Split(string(b), "\n"))
}

func AddFail2banIgnore(addr string) error {
	cur, err := ReadFail2banIgnore()
	if err != nil {
		return err
	}
	return SetFail2banIgnore(append(cur, addr))
}

func DelFail2banIgnore(addr string) error {
	cur, err := ReadFail2banIgnore()
	if err != nil {
		return err
	}
	one, err := NormalizeIgnore([]string{addr})
	if err != nil {
		return err
	}
	if len(one) == 0 {
		return fmt.Errorf("address required")
	}
	next := make([]string, 0, len(cur))
	for _, a := range cur {
		if a != one[0] {
			next = append(next, a)
		}
	}
	return SetFail2banIgnore(next)
}

func SetFail2banIgnore(addrs []string) error {
	norm, err := NormalizeIgnore(addrs)
	if err != nil {
		return err
	}
	body := strings.Join(norm, "\n")
	if body != "" {
		body += "\n"
	}
	if err := os.MkdirAll("/var/lib/siroc", 0750); err != nil {
		return err
	}
	if err := os.WriteFile(fail2banIgnoreStore, []byte(body), 0640); err != nil {
		return err
	}
	if _, err := exec.LookPath("fail2ban-client"); err != nil {
		return nil
	}
	if err := writeFail2banIgnoreJail(); err != nil {
		return err
	}
	if exec.Command("systemctl", "is-active", "--quiet", "fail2ban").Run() != nil {
		return nil
	}
	if out, err := exec.Command("fail2ban-client", "reload").CombinedOutput(); err != nil {
		return fmt.Errorf("fail2ban reload: %s", strings.TrimSpace(string(out)))
	}
	unbanIgnored(norm)
	return nil
}

func writeFail2banIgnoreJail() error {
	addrs, err := ReadFail2banIgnore()
	if err != nil {
		return err
	}
	body, err := RenderFail2banIgnore(addrs)
	if err != nil {
		return err
	}
	if err := os.MkdirAll("/etc/fail2ban/jail.d", 0755); err != nil {
		return err
	}
	return os.WriteFile(fail2banIgnoreJail, []byte(body), 0644)
}

func unbanIgnored(addrs []string) {
	if len(addrs) == 0 {
		return
	}
	out, err := exec.Command("fail2ban-client", "status").CombinedOutput()
	if err != nil {
		return
	}
	for _, name := range fail2banJailNames(string(out)) {
		bout, err := exec.Command("fail2ban-client", "status", name).CombinedOutput()
		if err != nil {
			continue
		}
		for _, ip := range fail2banBannedIPs(string(bout)) {
			if IPIgnored(ip, addrs) {
				_ = exec.Command("fail2ban-client", "set", name, "unbanip", ip).Run()
			}
		}
	}
}

func fail2banJailNames(status string) []string {
	var names []string
	for _, line := range strings.Split(status, "\n") {
		if !strings.Contains(line, "Jail list:") {
			continue
		}
		part := line[strings.Index(line, "Jail list:")+len("Jail list:"):]
		for _, name := range strings.Split(part, ",") {
			name = strings.TrimSpace(name)
			if name != "" && !strings.ContainsAny(name, " \t/\\") {
				names = append(names, name)
			}
		}
	}
	return names
}

func fail2banBannedIPs(status string) []string {
	for _, line := range strings.Split(status, "\n") {
		low := strings.ToLower(line)
		i := strings.Index(low, "banned ip list:")
		if i < 0 {
			continue
		}
		return strings.Fields(line[i+len("banned ip list:"):])
	}
	return nil
}

func fail2banJailSpecs(panelPort string) []JailSpec {
	specs := []JailSpec{{
		Name:   "siroc",
		Filter: "siroc",
		Logs:   []string{auth.AuthLogPath},
		Port:   panelPort + ",80,443",
	}}
	if log := firstLog("/var/log/auth.log", "/var/log/secure"); log != "" {
		specs = append(specs, JailSpec{Name: "sshd", Filter: "sshd", Logs: []string{log}, Port: "ssh"})
	}
	if installed("vsftpd") || fileExists("/etc/vsftpd.conf") {
		ensureFTPLog()
		logs := []string{"/var/log/vsftpd.log"}
		if fileExists("/var/log/auth.log") {
			logs = append(logs, "/var/log/auth.log")
		}
		specs = append(specs, JailSpec{Name: "vsftpd", Filter: pickFilter("vsftpd", "siroc-vsftpd"), Logs: logs, Port: "ftp,ftp-data,ftps,ftps-data"})
	}
	if log := mysqlLogPath(); log != "" {
		specs = append(specs, JailSpec{Name: "mysql", Filter: pickFilter("mysqld-auth", "siroc-mysql"), Logs: []string{log}, Port: "3306"})
	}
	if installed("redis-server") || fileExists("/var/log/redis/redis-server.log") || fileExists("/etc/redis/redis.conf") {
		if touchLog("/var/log/redis/redis-server.log") {
			specs = append(specs, JailSpec{Name: "redis", Filter: "siroc-redis", Logs: []string{"/var/log/redis/redis-server.log"}, Port: "6379"})
		}
	}
	if fileExists("/var/log/nginx/error.log") || installed("nginx") {
		if touchLog("/var/log/nginx/error.log") {
			specs = append(specs, JailSpec{Name: "nginx-http-auth", Filter: pickFilter("nginx-http-auth", "siroc-nginx"), Logs: []string{"/var/log/nginx/error.log"}, Port: "http,https"})
		}
	}
	if fileExists("/var/log/apache2/error.log") || installed("apache2") {
		if touchLog("/var/log/apache2/error.log") {
			specs = append(specs, JailSpec{Name: "apache-auth", Filter: pickFilter("apache-auth", "siroc-apache"), Logs: []string{"/var/log/apache2/error.log"}, Port: "http,https"})
		}
	}
	if fileExists("/etc/fail2ban/filter.d/recidive.conf") && touchLog("/var/log/fail2ban.log") {
		specs = append(specs, JailSpec{
			Name: "recidive", Filter: "recidive", Logs: []string{"/var/log/fail2ban.log"},
			Port: "0:65535", MaxRetry: 5, FindTime: "1d", BanTime: "1w",
		})
	}
	return specs
}

func writeExtraFilters() {
	filters := map[string]string{
		"/etc/fail2ban/filter.d/siroc-vsftpd.conf": `[Definition]
failregex = FAIL LOGIN: Client "<HOST>"
            authentication failure;.*rhost=<HOST>
ignoreregex =
`,
		"/etc/fail2ban/filter.d/siroc-mysql.conf": `[Definition]
failregex = Access denied for user .+@'<HOST>'
ignoreregex =
datepattern = %%Y-%%m-%%d[ T]%%H:%%M:%%S
              %%Y-%%m-%%dT%%H:%%M:%%S
`,
		"/etc/fail2ban/filter.d/siroc-redis.conf": `[Definition]
failregex = (?:Failed AUTH(?: attempt)?|AUTH failed).* from <HOST>
            Possible SECURITY ATTACK detected\..*<HOST>
ignoreregex =
datepattern = %%d %%b %%Y %%H:%%M:%%S
`,
		"/etc/fail2ban/filter.d/siroc-nginx.conf": `[Definition]
failregex = no user/password was provided for basic authentication.*client: <HOST>
            user .* was not found in .*client: <HOST>
            password mismatch, client: <HOST>
ignoreregex =
datepattern = %%Y/%%m/%%d %%H:%%M:%%S
`,
		"/etc/fail2ban/filter.d/siroc-apache.conf": `[Definition]
failregex = user .* (?:not found|password mismatch|authentication failure).*client <HOST>
            client denied by server configuration:.*\[client <HOST>
ignoreregex =
`,
	}
	for path, body := range filters {
		_ = os.WriteFile(path, []byte(body), 0644)
	}
}

func ensureServiceLogs() {
	if installed("mysql") || installed("mariadbd") || installed("mysqld") || fileExists("/etc/mysql/my.cnf") {
		_ = os.MkdirAll("/var/log/mysql", 0750)
		_ = os.MkdirAll("/etc/mysql/conf.d", 0755)
		_ = os.WriteFile("/etc/mysql/conf.d/siroc-log.cnf", []byte("[mysqld]\nlog_error=/var/log/mysql/error.log\n"), 0644)
		_ = touchLog("/var/log/mysql/error.log")
	}
}

func ensureFTPLog() {
	_ = touchLog("/var/log/vsftpd.log")
	b, err := os.ReadFile("/etc/vsftpd.conf")
	if err != nil || strings.Contains(string(b), "vsftpd_log_file=") {
		return
	}
	extra := "vsftpd_log_file=/var/log/vsftpd.log\ndual_log_enable=YES\n"
	if os.WriteFile("/etc/vsftpd.conf", append(b, []byte(extra)...), 0644) == nil {
		_ = exec.Command("systemctl", "reload", "vsftpd").Run()
	}
}

func mysqlLogPath() string {
	if !installed("mysql") && !installed("mariadbd") && !installed("mysqld") && !fileExists("/etc/mysql/my.cnf") && !fileExists("/var/log/mysql/error.log") && !fileExists("/var/log/mariadb/mariadb.log") {
		return ""
	}
	if p := firstLog("/var/log/mysql/error.log", "/var/log/mariadb/mariadb.log"); p != "" {
		return p
	}
	if touchLog("/var/log/mysql/error.log") {
		return "/var/log/mysql/error.log"
	}
	return ""
}

func pickFilter(stock, custom string) string {
	if fileExists("/etc/fail2ban/filter.d/" + stock + ".conf") {
		return stock
	}
	return custom
}

func firstLog(paths ...string) string {
	for _, p := range paths {
		if fileExists(p) {
			return p
		}
	}
	return ""
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func installed(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}

func touchLog(path string) bool {
	if fileExists(path) {
		return true
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return false
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

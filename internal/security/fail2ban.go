//go:build linux

package security

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"

	"github.com/siroc-dev/siroc/internal/auth"
)

const (
	fail2banFilter = "/etc/fail2ban/filter.d/siroc.conf"
	fail2banJail   = "/etc/fail2ban/jail.d/siroc.conf"
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
	port := CPWebPort()
	jail := fmt.Sprintf(`[siroc]
enabled = true
filter = siroc
logpath = %s
backend = polling
port = %s,80,443
maxretry = 5
findtime = 10m
bantime = 1h
`, auth.AuthLogPath, port)
	if err := os.WriteFile(fail2banJail, []byte(jail), 0644); err != nil {
		return err
	}
	if _, err := os.Stat("/etc/fail2ban/jail.local"); err != nil {
		_ = os.WriteFile("/etc/fail2ban/jail.local", []byte("[DEFAULT]\nbantime = 1h\nfindtime = 10m\nmaxretry = 5\n\n[sshd]\nenabled = true\n"), 0644)
	}
	_ = exec.Command("systemctl", "enable", "--now", "fail2ban").Run()
	if exec.Command("systemctl", "is-active", "--quiet", "fail2ban").Run() == nil {
		_ = exec.Command("fail2ban-client", "reload").Run()
	}
	return nil
}

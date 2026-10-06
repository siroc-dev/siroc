//go:build linux

package software

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/siroc-dev/siroc/internal/validate"
)

const (
	vsftpdConf     = "/etc/vsftpd.conf"
	vsftpdUserList = "/etc/vsftpd.user_list"
	sshdDropInDir  = "/etc/ssh/sshd_config.d"
	sshdDropIn     = "/etc/ssh/sshd_config.d/siroc.conf"
)

func configureVSFTPD(string) error {
	if err := os.WriteFile("/etc/pam.d/siroc", []byte(vsftpdPAM()), 0644); err != nil {
		return err
	}
	_ = allowFTPShell()
	changed, err := writeFileIfChanged(vsftpdConf, []byte(vsftpdConfig(os.Getenv("SIROC_FTP_PASV_ADDR"))), 0644)
	if err != nil {
		return err
	}
	if err := ensureFTPDenyFile(); err != nil {
		return err
	}
	_ = exec.Command("systemctl", "stop", "vsftpd.socket").Run()
	_ = exec.Command("systemctl", "disable", "vsftpd.socket").Run()
	_ = os.MkdirAll("/var/run/vsftpd/empty", 0755)
	_ = exec.Command("systemctl", "enable", "vsftpd").Run()
	if changed {
		return exec.Command("systemctl", "restart", "vsftpd").Run()
	}
	return exec.Command("systemctl", "start", "vsftpd").Run()
}

// allowFTPShell lists /usr/sbin/nologin in /etc/shells. Ubuntu's stock
// vsftpd PAM module rejects that shell, and the FTP client reports a bad password.
func allowFTPShell() error {
	const shells = "/etc/shells"
	const want = "/usr/sbin/nologin"
	b, err := os.ReadFile(shells)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) == want {
			return nil
		}
	}
	f, err := os.OpenFile(shells, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if len(b) > 0 && b[len(b)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	_, err = f.WriteString(want + "\n")
	return err
}

func writeFileIfChanged(path string, body []byte, mode os.FileMode) (bool, error) {
	old, err := os.ReadFile(path)
	if err == nil && string(old) == string(body) {
		return false, nil
	}
	if err := os.WriteFile(path, body, mode); err != nil {
		return false, err
	}
	return true, nil
}

func configureSSH(string) error {
	if err := os.MkdirAll(sshdDropInDir, 0755); err != nil {
		return err
	}
	conf := `PasswordAuthentication yes
PubkeyAuthentication yes
AllowTcpForwarding no
X11Forwarding no
`
	if err := os.WriteFile(sshdDropIn, []byte(conf), 0644); err != nil {
		return err
	}
	_ = exec.Command("ssh-keygen", "-A").Run()
	if err := exec.Command("systemctl", "enable", "--now", "ssh").Run(); err != nil {
		return exec.Command("systemctl", "enable", "--now", "sshd").Run()
	}
	_ = exec.Command("systemctl", "reload", "ssh").Run()
	return nil
}

func (m *Manager) EnsureAccess() error {
	if ok, _ := dpkgVersion("openssh-server"); !ok {
		if err := m.Install("openssh", ""); err != nil {
			return fmt.Errorf("openssh: %w", err)
		}
	} else {
		_ = configureSSH("")
	}
	if ok, _ := dpkgVersion("vsftpd"); !ok {
		if err := m.Install("vsftpd", ""); err != nil {
			return fmt.Errorf("vsftpd: %w", err)
		}
	} else {
		_ = configureVSFTPD("")
	}
	return nil
}

func SetFTPAllowed(username string, allowed bool) error {
	if err := validate.LinuxUser(username); err != nil {
		return err
	}
	if err := ensureFTPDenyFile(); err != nil {
		return err
	}
	raw, err := os.ReadFile(vsftpdUserList)
	if err != nil {
		return err
	}
	var keep []string
	seen := false
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			if line != "" {
				keep = append(keep, line)
			}
			continue
		}
		if line == username {
			seen = true
			if allowed {
				continue
			}
		}
		keep = append(keep, line)
	}
	if !allowed && !seen {
		keep = append(keep, username)
	}
	body := strings.Join(keep, "\n") + "\n"
	if err := os.WriteFile(vsftpdUserList, []byte(body), 0644); err != nil {
		return err
	}
	_ = exec.Command("systemctl", "reload", "vsftpd").Run()
	return nil
}

func ensureFTPDenyFile() error {
	if _, err := os.Stat(vsftpdUserList); err == nil {
		return nil
	}
	return os.WriteFile(vsftpdUserList, []byte("root\nnobody\nsiroc\n"), 0644)
}

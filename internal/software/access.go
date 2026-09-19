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
	pasv := os.Getenv("SIROC_FTP_PASV_ADDR")
	conf := `listen=YES
listen_ipv6=NO
anonymous_enable=NO
local_enable=YES
write_enable=YES
local_umask=022
dirmessage_enable=YES
use_localtime=YES
xferlog_enable=YES
connect_from_port_20=YES
chroot_local_user=YES
allow_writeable_chroot=YES
userlist_enable=YES
userlist_deny=YES
userlist_file=/etc/vsftpd.user_list
secure_chroot_dir=/var/run/vsftpd/empty
pam_service_name=vsftpd
pasv_enable=YES
pasv_min_port=30000
pasv_max_port=30100
seccomp_sandbox=NO
`
	if pasv != "" {
		conf += "pasv_address=" + pasv + "\n"
	}
	if err := os.WriteFile(vsftpdConf, []byte(conf), 0644); err != nil {
		return err
	}
	if err := ensureFTPDenyFile(); err != nil {
		return err
	}
	_ = exec.Command("systemctl", "stop", "vsftpd.socket").Run()
	_ = exec.Command("systemctl", "disable", "vsftpd.socket").Run()
	_ = os.MkdirAll("/var/run/vsftpd/empty", 0755)
	return exec.Command("systemctl", "enable", "--now", "vsftpd").Run()
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

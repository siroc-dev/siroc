//go:build linux

package software

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func fetchReleaseComponents(mirror, suite string) ([]string, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	var last error
	for _, u := range releaseURLs(mirror, suite) {
		res, err := client.Get(u)
		if err != nil {
			last = err
			continue
		}
		body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		_ = res.Body.Close()
		if err != nil {
			last = err
			continue
		}
		if res.StatusCode != http.StatusOK {
			last = fmt.Errorf("%s: HTTP %d", u, res.StatusCode)
			continue
		}
		comps := parseReleaseComponents(string(body))
		if len(comps) == 0 {
			last = fmt.Errorf("%s: no components", u)
			continue
		}
		return comps, nil
	}
	if last == nil {
		last = fmt.Errorf("no release file")
	}
	return nil, fmt.Errorf("read %s %s: %w", mirror, suite, last)
}

func writeMySQLKeyring() error {
	dest := "/etc/apt/keyrings/cp-mysql.gpg"
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	var files []string
	defer func() {
		for _, f := range files {
			_ = os.Remove(f)
		}
	}()
	for _, u := range []string{
		"https://repo.mysql.com/RPM-GPG-KEY-mysql-2025",
		"https://repo.mysql.com/RPM-GPG-KEY-mysql-2023",
	} {
		tmp, err := os.CreateTemp(ensureDiskTmp(), "siroc-mysql-key-*")
		if err != nil {
			return err
		}
		tmpName := tmp.Name()
		_ = tmp.Close()
		dl := exec.Command("curl", "-fsSL", "-o", tmpName, u)
		if out, err := combinedTimeout(dl, 45*time.Second); err != nil {
			_ = os.Remove(tmpName)
			return fmt.Errorf("download MySQL key: %s: %w", tail(out), err)
		}
		files = append(files, tmpName)
	}
	_ = os.Remove(dest)
	args := append([]string{"--batch", "--yes", "--no-default-keyring", "--keyring", dest, "--import"}, files...)
	if out, err := exec.Command("gpg", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("import MySQL key: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return os.Chmod(dest, 0644)
}

func retireSQLSource() {
	stopSQLServices()
	for _, name := range []string{"mysql", "mariadb"} {
		unit := "/etc/systemd/system/" + name + ".service"
		if b, err := os.ReadFile(unit); err == nil && strings.Contains(string(b), "/opt/siroc/db") {
			_ = os.Remove(unit)
		}
		_ = os.RemoveAll(sqlPrefix(name))
	}
	for _, name := range []string{"mysql", "mysqld", "mysqladmin", "mysqldump", "mariadb", "mariadbd", "mariadb-admin", "mariadb-dump"} {
		dest := filepath.Join("/usr/bin", name)
		target, err := os.Readlink(dest)
		if err == nil && strings.Contains(target, "/opt/siroc/db") {
			_ = os.Remove(dest)
		}
	}
	if b, err := os.ReadFile("/etc/mysql/my.cnf"); err == nil && strings.Contains(string(b), "/opt/siroc/db") {
		_ = os.Remove("/etc/mysql/my.cnf")
	}
	_ = exec.Command("systemctl", "daemon-reload").Run()
}

func startSQLService(names ...string) error {
	_ = exec.Command("systemctl", "daemon-reload").Run()
	var last error
	for _, name := range names {
		out, err := exec.Command("systemctl", "enable", "--now", name).CombinedOutput()
		if err == nil {
			return nil
		}
		last = fmt.Errorf("start %s: %s: %w", name, strings.TrimSpace(string(out)), err)
	}
	return last
}

func randomAptPassword() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func preseedMySQLRoot(pass string) {
	body := "mysql-community-server mysql-community-server/root-pass password " + pass + "\n" +
		"mysql-community-server mysql-community-server/re-root-pass password " + pass + "\n"
	cmd := exec.Command("debconf-set-selections")
	cmd.Stdin = strings.NewReader(body)
	cmd.Env = aptEnv()
	_, _ = combinedTimeout(cmd, 20*time.Second)
}

func ensureSQLLocalRoot(password string) error {
	time.Sleep(2 * time.Second)
	if sqlAdminOK() {
		return nil
	}
	if password != "" {
		cnf, err := writeRootClientCnf(password)
		if err == nil {
			_ = switchRootToSocket(cnf)
			_ = os.Remove(cnf)
		}
	}
	if sqlAdminOK() {
		return nil
	}
	if _, err := os.Stat("/etc/mysql/debian.cnf"); err == nil {
		_ = switchRootToSocket("/etc/mysql/debian.cnf")
	}
	if sqlAdminOK() {
		return nil
	}
	return fmt.Errorf("database root cannot log in through the local socket")
}

func writeRootClientCnf(password string) (string, error) {
	f, err := os.CreateTemp("", "siroc-root-*.cnf")
	if err != nil {
		return "", err
	}
	name := f.Name()
	body := "[client]\nuser=root\npassword=" + password + "\n"
	if _, err := f.WriteString(body); err != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := f.Chmod(0600); err != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return "", err
	}
	_ = f.Close()
	return name, nil
}

func sqlClientBin() string {
	if _, err := exec.LookPath("mysql"); err == nil {
		return "mysql"
	}
	return "mariadb"
}

func sqlAdminOK() bool {
	bin := sqlClientBin()
	for _, flags := range [][]string{
		{"--user=root", "--batch", "--skip-ssl", "-e", "SELECT 1"},
		{"--user=root", "--batch", "-e", "SELECT 1"},
	} {
		cmd := exec.Command(bin, flags...)
		cmd.Env = os.Environ()
		if cmd.Run() == nil {
			return true
		}
	}
	return false
}

func switchRootToSocket(defaultsFile string) error {
	bin := sqlClientBin()
	base := []string{"--defaults-extra-file=" + defaultsFile, "--batch"}
	for _, sql := range []string{
		"ALTER USER 'root'@'localhost' IDENTIFIED WITH auth_socket",
		"ALTER USER 'root'@'localhost' IDENTIFIED VIA unix_socket",
		"FLUSH PRIVILEGES",
	} {
		cmd := exec.Command(bin, append(base, "-e", sql)...)
		cmd.Env = os.Environ()
		_ = cmd.Run()
	}
	if sqlAdminOK() {
		return nil
	}
	return fmt.Errorf("socket login failed")
}

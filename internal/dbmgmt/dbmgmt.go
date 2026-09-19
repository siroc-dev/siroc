//go:build linux

package dbmgmt

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/validate"
)

type Manager struct{}

func mysqlBin() string {
	if _, err := exec.LookPath("mariadb"); err == nil {
		return "mariadb"
	}
	return "mysql"
}

func (m *Manager) Engine() string {
	bin := mysqlBin()
	if exec.Command(bin, "--version").Run() != nil {
		return ""
	}
	out, _ := exec.Command(bin, "--version").CombinedOutput()
	s := strings.ToLower(string(out))
	if strings.Contains(s, "mariadb") {
		return "mariadb"
	}
	if strings.Contains(s, "mysql") {
		return "mysql"
	}
	return ""
}

func (m *Manager) Create(req rpc.DBCreateReq) error {
	if err := validate.DBIdent(req.DBName); err != nil {
		return err
	}
	if err := validate.DBIdent(req.DBUser); err != nil {
		return err
	}
	if len(req.Password) < 8 {
		return fmt.Errorf("database password must be at least 8 characters")
	}
	if m.Engine() == "" {
		return fmt.Errorf("MySQL/MariaDB is not installed")
	}
	sql := strings.Join([]string{
		fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`;", req.DBName),
		fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'localhost' IDENTIFIED BY '%s';", escape(req.DBUser), escape(req.Password)),
		fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'localhost';", req.DBName, escape(req.DBUser)),
		"FLUSH PRIVILEGES;",
	}, "\n")
	return mysqlExec(sql)
}

func (m *Manager) Drop(req rpc.DBDropReq) error {
	if err := validate.DBIdent(req.DBName); err != nil {
		return err
	}
	if req.DBUser != "" {
		if err := validate.DBIdent(req.DBUser); err != nil {
			return err
		}
	}
	if m.Engine() == "" {
		return fmt.Errorf("MySQL/MariaDB is not installed")
	}
	parts := []string{fmt.Sprintf("DROP DATABASE IF EXISTS `%s`;", req.DBName)}
	if req.DBUser != "" {
		parts = append(parts, fmt.Sprintf("DROP USER IF EXISTS '%s'@'localhost';", escape(req.DBUser)))
	}
	parts = append(parts, "FLUSH PRIVILEGES;")
	return mysqlExec(strings.Join(parts, "\n"))
}

func (m *Manager) SetPassword(user, password string) error {
	if err := validate.DBIdent(user); err != nil {
		return err
	}
	if len(password) < 8 {
		return fmt.Errorf("database password must be at least 8 characters")
	}
	if m.Engine() == "" {
		return fmt.Errorf("MySQL/MariaDB is not installed")
	}
	sql := fmt.Sprintf("ALTER USER '%s'@'localhost' IDENTIFIED BY '%s';\nFLUSH PRIVILEGES;", escape(user), escape(password))
	return mysqlExec(sql)
}

func mysqlExec(sql string) error {
	bin := mysqlBin()
	flagSets := [][]string{
		{"--batch", "--raw", "--skip-ssl"},
		{"--batch", "--raw", "--ssl-mode=DISABLED"},
		{"--batch", "--raw"},
	}
	var last error
	for _, flags := range flagSets {
		cmd := exec.Command(bin, flags...)
		cmd.Stdin = strings.NewReader(sql)
		cmd.Env = os.Environ()
		out, err := cmd.CombinedOutput()
		if err == nil {
			return nil
		}
		last = fmt.Errorf("mysql: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return last
}

func escape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return s
}

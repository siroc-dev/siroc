//go:build linux

package dbmgmt

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/sqlpack"
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
	sql := strings.Join(append(
		[]string{fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`;", req.DBName)},
		userHostSQL(req.DBUser, req.Password, req.DBName)...,
	), "\n")
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
		for _, host := range clientHosts {
			parts = append(parts, fmt.Sprintf("DROP USER IF EXISTS '%s'@'%s';", escape(req.DBUser), host))
		}
	}
	parts = append(parts, "FLUSH PRIVILEGES;")
	return mysqlExec(strings.Join(parts, "\n"))
}

func (m *Manager) SetPassword(user, password, dbName string) error {
	if err := validate.DBIdent(user); err != nil {
		return err
	}
	if dbName != "" {
		if err := validate.DBIdent(dbName); err != nil {
			return err
		}
	}
	if len(password) < 8 {
		return fmt.Errorf("database password must be at least 8 characters")
	}
	if m.Engine() == "" {
		return fmt.Errorf("MySQL/MariaDB is not installed")
	}
	return mysqlExec(strings.Join(userHostSQL(user, password, dbName), "\n"))
}

// clientHosts covers the socket login (localhost) and the TCP login phpMyAdmin uses (127.0.0.1).
var clientHosts = []string{"localhost", "127.0.0.1"}

func userHostSQL(user, password, dbName string) []string {
	var parts []string
	for _, host := range clientHosts {
		parts = append(parts,
			fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%s' IDENTIFIED BY '%s';", escape(user), host, escape(password)),
			fmt.Sprintf("ALTER USER '%s'@'%s' IDENTIFIED BY '%s';", escape(user), host, escape(password)),
		)
		if dbName != "" {
			parts = append(parts, fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'%s';", dbName, escape(user), host))
		}
	}
	parts = append(parts, "FLUSH PRIVILEGES;")
	return parts
}

func (m *Manager) Dump(name string) (*os.File, error) {
	if err := validate.DBIdent(name); err != nil {
		return nil, err
	}
	if m.Engine() == "" {
		return nil, fmt.Errorf("MySQL/MariaDB is not installed")
	}
	dumpBin := "mysqldump"
	if _, err := exec.LookPath("mariadb-dump"); err == nil {
		dumpBin = "mariadb-dump"
	}
	f, err := os.CreateTemp("", "siroc-dump-*.sql")
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	var dumpErr error
	for _, extra := range [][]string{{"--skip-ssl"}, {"--ssl-mode=DISABLED"}, nil} {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			_ = f.Close()
			_ = os.Remove(f.Name())
			return nil, err
		}
		_ = f.Truncate(0)
		stderr.Reset()
		args := append([]string{"--single-transaction", "--routines", "--triggers", "--default-character-set=utf8mb4"}, extra...)
		args = append(args, name)
		cmd := exec.Command(dumpBin, args...)
		cmd.Stdout = f
		cmd.Stderr = &stderr
		dumpErr = cmd.Run()
		if dumpErr == nil {
			break
		}
	}
	if dumpErr != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = dumpErr.Error()
		}
		return nil, fmt.Errorf("export %s: %s", name, msg)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return nil, err
	}
	return f, nil
}

func (m *Manager) Import(name, user, password, filename string, r io.Reader) error {
	if err := validate.DBIdent(name); err != nil {
		return err
	}
	if err := validate.DBIdent(user); err != nil {
		return err
	}
	if strings.ContainsAny(password, "\r\n") || password == "" {
		return fmt.Errorf("database password is missing; reset it and try again")
	}
	if m.Engine() == "" {
		return fmt.Errorf("MySQL/MariaDB is not installed")
	}
	done := beginImportLog(name, filename)
	err := m.importStream(name, user, password, filename, r)
	done(err)
	return err
}

func (m *Manager) importStream(name, user, password, filename string, r io.Reader) error {
	noteImport(name, "receiving "+filepath.Base(filename))
	recv := &byteNote{r: r, name: name, label: "received", every: 32 << 20, next: 32 << 20}
	sql, err := sqlpack.Open(filename, recv)
	if err != nil {
		return err
	}
	defer sql.Close()
	noteImport(name, "received "+formatLogBytes(recv.n))
	if err := mysqlExec("CREATE DATABASE IF NOT EXISTS `" + name + "`;"); err != nil {
		return err
	}
	cnf, err := os.CreateTemp("", "siroc-db-*.cnf")
	if err != nil {
		return err
	}
	cnfName := cnf.Name()
	defer os.Remove(cnfName)
	body := "[client]\nuser=" + optionValue(user) + "\npassword=" + optionValue(password) + "\n"
	if _, err := cnf.WriteString(body); err != nil {
		_ = cnf.Close()
		return err
	}
	if err := cnf.Chmod(0600); err != nil {
		_ = cnf.Close()
		return err
	}
	_ = cnf.Close()
	noteImport(name, "mysql --default-character-set=utf8mb4 "+name)
	fed := &byteNote{r: sqlpack.Sanitize(sql), name: name, label: "imported", every: 32 << 20, next: 32 << 20}
	if err := runMysqlImport(name, cnfName, fed); err != nil {
		return err
	}
	noteImport(name, "imported "+formatLogBytes(fed.n))
	return nil
}

func runMysqlImport(name, cnfName string, in io.Reader) error {
	cmd := exec.Command(mysqlBin(), "--defaults-extra-file="+cnfName, "--default-character-set=utf8mb4", name)
	cmd.Stdin = in
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw
	readDone := make(chan struct{})
	var tail strings.Builder
	go func() {
		defer close(readDone)
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			noteImport(name, line)
			if tail.Len() > 8192 {
				s := tail.String()
				tail.Reset()
				if len(s) > 4096 {
					s = s[len(s)-4096:]
				}
				tail.WriteString(s)
			}
			tail.WriteString(line)
			tail.WriteByte('\n')
		}
	}()
	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		<-readDone
		return err
	}
	err := cmd.Wait()
	_ = pw.Close()
	<-readDone
	if err != nil {
		msg := strings.TrimSpace(tail.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("import %s: %s", name, msg)
	}
	return nil
}

func (m *Manager) Sizes() (map[string]int64, error) {
	out := map[string]int64{}
	if m.Engine() == "" {
		return out, nil
	}
	sql := "SELECT table_schema, CAST(IFNULL(SUM(data_length + index_length), 0) AS UNSIGNED) FROM information_schema.tables WHERE table_schema NOT IN ('information_schema','mysql','performance_schema','sys') GROUP BY table_schema"
	raw, err := mysqlQuery(sql)
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		n, _ := strconv.ParseInt(fields[1], 10, 64)
		out[fields[0]] = n
	}
	return out, nil
}

func optionValue(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func mysqlQuery(sql string) (string, error) {
	bin := mysqlBin()
	flagSets := [][]string{
		{"--batch", "--raw", "--skip-column-names", "--skip-ssl"},
		{"--batch", "--raw", "--skip-column-names", "--ssl-mode=DISABLED"},
		{"--batch", "--raw", "--skip-column-names"},
	}
	var last error
	for _, flags := range flagSets {
		cmd := exec.Command(bin, append(flags, "-e", sql)...)
		cmd.Env = os.Environ()
		out, err := cmd.Output()
		if err == nil {
			return string(out), nil
		}
		last = err
	}
	return "", fmt.Errorf("mysql: %w", last)
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

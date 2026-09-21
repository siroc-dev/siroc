//go:build linux

package software

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	sqlSourceRoot = "/opt/siroc/db"
	sqlDataDir    = "/var/lib/mysql"
	sqlRunDir     = "/run/mysqld"
	sqlConfDir    = "/etc/mysql/conf.d"
)

func sqlPrefix(engine string) string {
	return filepath.Join(sqlSourceRoot, engine)
}

func sqlServerBin(engine string) string {
	if engine == "mariadb" {
		return filepath.Join(sqlPrefix(engine), "bin", "mariadbd")
	}
	return filepath.Join(sqlPrefix(engine), "bin", "mysqld")
}

func sqlSourceInstalled(engine string) (bool, string) {
	if _, err := os.Stat(sqlServerBin(engine)); err != nil {
		return false, ""
	}
	if b, err := os.ReadFile(filepath.Join(sqlPrefix(engine), "SIROC_VERSION")); err == nil {
		if v := strings.TrimSpace(string(b)); v != "" {
			return true, v
		}
	}
	out, _ := exec.Command(sqlServerBin(engine), "--version").CombinedOutput()
	return true, strings.TrimSpace(string(out))
}

func resolveMariaDBFull(series string) string {
	body, err := httpGet("https://downloads.mariadb.org/rest-api/mariadb/"+series+"/", 20*time.Second)
	if err == nil {
		if v := parseMariaDBLatest(body, series); v != "" {
			return v
		}
	}
	return mariadbSourceFallback[series]
}

func resolveMySQLFull(series string) string {
	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/mysql/mysql-server/tags?per_page=100", nil)
	if err == nil {
		req.Header.Set("Accept", "application/vnd.github+json")
		client := &http.Client{Timeout: 20 * time.Second}
		if res, err := client.Do(req); err == nil {
			body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
			_ = res.Body.Close()
			if v := parseMySQLTagLatest(body, series); v != "" {
				return v
			}
		}
	}
	return mysqlSourceFallback[series]
}

func httpGet(url string, d time.Duration) ([]byte, error) {
	client := &http.Client{Timeout: d}
	res, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", url, res.StatusCode)
	}
	return io.ReadAll(io.LimitReader(res.Body, 2<<20))
}

func installSQLBuildDeps() error {
	return aptInstall(
		"build-essential", "cmake", "ninja-build", "bison", "pkg-config",
		"ca-certificates", "curl", "libssl-dev", "zlib1g-dev", "libncurses-dev",
		"libpcre2-dev", "libzstd-dev", "liblz4-dev", "libsnappy-dev",
		"libcurl4-openssl-dev", "libxml2-dev", "libaio-dev", "libnuma-dev",
		"libsystemd-dev",
	)
}

func ensureMysqlUser() error {
	if _, err := exec.Command("id", "-u", "mysql").CombinedOutput(); err == nil {
		return nil
	}
	_ = exec.Command("groupadd", "-r", "mysql").Run()
	out, err := exec.Command("useradd", "-r", "-g", "mysql", "-s", "/usr/sbin/nologin", "-d", sqlDataDir, "mysql").CombinedOutput()
	if err != nil {
		return fmt.Errorf("useradd mysql: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func downloadSQLTarball(url, dest string) error {
	_ = os.MkdirAll(filepath.Dir(dest), 0755)
	cmd := exec.Command("curl", "-fL", "--retry", "3", "-o", dest, url)
	if out, err := combinedTimeout(cmd, 15*time.Minute); err != nil {
		return fmt.Errorf("download %s: %s: %w", url, tail(out), err)
	}
	return nil
}

func extractSQLTarball(tgz, dest string) (string, error) {
	_ = os.RemoveAll(dest)
	if err := os.MkdirAll(dest, 0755); err != nil {
		return "", err
	}
	if out, err := combinedTimeout(exec.Command("tar", "-xzf", tgz, "-C", dest, "--strip-components=1"), 5*time.Minute); err != nil {
		return "", fmt.Errorf("extract: %s: %w", tail(out), err)
	}
	return dest, nil
}

func cmakeSQL(engine, src, prefix string) error {
	build := filepath.Join(src, "siroc-build")
	if err := os.MkdirAll(build, 0755); err != nil {
		return err
	}
	args := []string{
		"-S", src, "-B", build, "-G", "Ninja",
		"-DCMAKE_BUILD_TYPE=RelWithDebInfo",
		"-DCMAKE_INSTALL_PREFIX=" + prefix,
		"-DWITH_SSL=system",
		"-DWITH_ZLIB=system",
		"-DWITH_UNIT_TESTS=OFF",
		"-DWITH_EMBEDDED_SERVER=OFF",
		"-DENABLED_LOCAL_INFILE=ON",
	}
	if engine == "mariadb" {
		args = append(args,
			"-DPLUGIN_TOKUDB=NO",
			"-DPLUGIN_MROONGA=NO",
			"-DPLUGIN_ROCKSDB=NO",
			"-DPLUGIN_SPIDER=NO",
			"-DPLUGIN_OQGRAPH=NO",
			"-DPLUGIN_CONNECT=NO",
			"-DPLUGIN_S3=NO",
			"-DPLUGIN_COLUMNSTORE=NO",
		)
	} else {
		args = append(args,
			"-DDOWNLOAD_BOOST=1",
			"-DWITH_BOOST="+filepath.Join(src, "boost"),
			"-DWITH_AUTHENTICATION_FIDO=OFF",
		)
	}
	cmd := exec.Command("cmake", args...)
	cmd.Dir = src
	if out, err := combinedTimeout(cmd, 15*time.Minute); err != nil {
		return fmt.Errorf("cmake: %s: %w", tail(out), err)
	}
	jobs := strconv.Itoa(runtime.NumCPU())
	if jobs == "0" {
		jobs = "2"
	}
	buildCmd := exec.Command("cmake", "--build", build, "-j", jobs)
	if out, err := combinedTimeout(buildCmd, 2*time.Hour); err != nil {
		return fmt.Errorf("compile: %s: %w", tail(out), err)
	}
	inst := exec.Command("cmake", "--install", build)
	if out, err := combinedTimeout(inst, 10*time.Minute); err != nil {
		return fmt.Errorf("install: %s: %w", tail(out), err)
	}
	return nil
}

func linkSQLBins(engine string) {
	binDir := filepath.Join(sqlPrefix(engine), "bin")
	names := []string{"mysql", "mysqld", "mysqladmin", "mysqldump", "mariadb", "mariadbd", "mariadb-admin", "mariadb-dump"}
	for _, name := range names {
		src := filepath.Join(binDir, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dest := filepath.Join("/usr/bin", name)
		_ = os.Remove(dest)
		_ = os.Symlink(src, dest)
	}
}

func writeSQLConfig(engine string) error {
	if err := os.MkdirAll(sqlConfDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll("/etc/mysql", 0755); err != nil {
		return err
	}
	body := fmt.Sprintf(`[client]
socket=%s/mysqld.sock

[mysqld]
user=mysql
basedir=%s
datadir=%s
socket=%s/mysqld.sock
pid-file=%s/mysqld.pid
bind-address=127.0.0.1
!includedir %s/
`, sqlRunDir, sqlPrefix(engine), sqlDataDir, sqlRunDir, sqlRunDir, sqlConfDir)
	return os.WriteFile("/etc/mysql/my.cnf", []byte(body), 0644)
}

func writeSQLService(engine string) error {
	bin := sqlServerBin(engine)
	name := "mysql"
	desc := "MySQL (Siroc source build)"
	if engine == "mariadb" {
		name = "mariadb"
		desc = "MariaDB (Siroc source build)"
	}
	unit := fmt.Sprintf(`[Unit]
Description=%s
After=network.target

[Service]
Type=simple
User=mysql
Group=mysql
RuntimeDirectory=mysqld
RuntimeDirectoryMode=0755
ExecStart=%s --defaults-file=/etc/mysql/my.cnf
Restart=on-failure
LimitNOFILE=65535
TimeoutStartSec=120

[Install]
WantedBy=multi-user.target
`, desc, bin)
	path := "/etc/systemd/system/" + name + ".service"
	if err := os.WriteFile(path, []byte(unit), 0644); err != nil {
		return err
	}
	_ = exec.Command("systemctl", "daemon-reload").Run()
	return nil
}

func initSQLDatadir(engine string) error {
	if err := os.MkdirAll(sqlDataDir, 0750); err != nil {
		return err
	}
	ents, _ := os.ReadDir(sqlDataDir)
	if len(ents) > 0 {
		_ = exec.Command("chown", "-R", "mysql:mysql", sqlDataDir).Run()
		return nil
	}
	prefix := sqlPrefix(engine)
	var cmd *exec.Cmd
	if engine == "mariadb" {
		script := filepath.Join(prefix, "scripts", "mariadb-install-db")
		if _, err := os.Stat(script); err != nil {
			script = filepath.Join(prefix, "bin", "mariadb-install-db")
		}
		cmd = exec.Command(script, "--user=mysql", "--basedir="+prefix, "--datadir="+sqlDataDir)
	} else {
		cmd = exec.Command(sqlServerBin(engine), "--defaults-file=/etc/mysql/my.cnf", "--initialize-insecure", "--user=mysql")
	}
	if out, err := combinedTimeout(cmd, 5*time.Minute); err != nil {
		return fmt.Errorf("initialize data dir: %s: %w", tail(out), err)
	}
	_ = exec.Command("chown", "-R", "mysql:mysql", sqlDataDir).Run()
	return nil
}

func secureSQLRoot(engine string) {
	bin := "mysql"
	if engine == "mariadb" {
		if _, err := exec.LookPath("mariadb"); err == nil {
			bin = "mariadb"
		}
	}
	stmts := []string{
		"ALTER USER 'root'@'localhost' IDENTIFIED VIA unix_socket;",
		"ALTER USER 'root'@'localhost' IDENTIFIED WITH auth_socket;",
		"FLUSH PRIVILEGES;",
	}
	for _, sql := range stmts {
		_ = exec.Command(bin, "--user=root", "--batch", "-e", sql).Run()
	}
}

func buildSQLFromSource(engine, series string) error {
	var full, url, alt string
	if engine == "mariadb" {
		full = resolveMariaDBFull(series)
		if full == "" {
			return fmt.Errorf("MariaDB %s source release not found", series)
		}
		url = mariadbSourceURL(full)
	} else {
		full = resolveMySQLFull(series)
		if full == "" {
			return fmt.Errorf("MySQL %s source release not found", series)
		}
		url = mysqlSourceURL(full)
		alt = mysqlSourceURLAlt(full)
	}
	if err := installSQLBuildDeps(); err != nil {
		return fmt.Errorf("build dependencies: %w", err)
	}
	if err := ensureMysqlUser(); err != nil {
		return err
	}
	srcRoot := filepath.Join("/opt/siroc/.src", engine+"-"+full)
	tgz := srcRoot + ".tar.gz"
	if err := downloadSQLTarball(url, tgz); err != nil && alt != "" {
		if err2 := downloadSQLTarball(alt, tgz); err2 != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	src, err := extractSQLTarball(tgz, srcRoot)
	if err != nil {
		return err
	}
	prefix := sqlPrefix(engine)
	_ = os.RemoveAll(prefix)
	if err := cmakeSQL(engine, src, prefix); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(prefix, "SIROC_VERSION"), []byte(full+"\n"), 0644); err != nil {
		return err
	}
	_ = exec.Command("chown", "-R", "root:root", prefix).Run()
	linkSQLBins(engine)
	if err := writeSQLConfig(engine); err != nil {
		return err
	}
	if err := writeSQLService(engine); err != nil {
		return err
	}
	if err := initSQLDatadir(engine); err != nil {
		return err
	}
	unit := "mysql"
	if engine == "mariadb" {
		unit = "mariadb"
	}
	if out, err := exec.Command("systemctl", "enable", "--now", unit).CombinedOutput(); err != nil {
		return fmt.Errorf("start %s: %s: %w", unit, strings.TrimSpace(string(out)), err)
	}
	time.Sleep(2 * time.Second)
	secureSQLRoot(engine)
	_ = os.Remove(tgz)
	return nil
}

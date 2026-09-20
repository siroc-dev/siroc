//go:build linux

package software

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func osRelease() (id, codename string) {
	id, codename = "ubuntu", "noble"
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return id, codename
	}
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"'`)
		switch k {
		case "ID":
			id = strings.ToLower(v)
		case "VERSION_CODENAME":
			codename = v
		}
	}
	if id == "linuxmint" || id == "pop" {
		id = "ubuntu"
	}
	return id, codename
}

const diskTmpDir = "/opt/siroc/.tmp"

func ensureDiskTmp() string {
	_ = os.MkdirAll(diskTmpDir, 0755)
	return diskTmpDir
}

func aptEnv() []string {
	tmp := ensureDiskTmp()
	return append(os.Environ(),
		"DEBIAN_FRONTEND=noninteractive",
		"DEBCONF_NONINTERACTIVE_SEEN=true",
		"APT_LISTCHANGES_FRONTEND=none",
		"NEEDRESTART_MODE=a",
		"UCF_FORCE_CONFFOLD=1",
		"TMPDIR="+tmp,
		"TMP="+tmp,
		"TEMP="+tmp,
	)
}

func aptOpts(action ...string) []string {
	args := []string{
		"-y",
		"-o", "Dpkg::Options::=--force-confdef",
		"-o", "Dpkg::Options::=--force-confold",
	}
	return append(args, action...)
}

func aptCmd(args ...string) *exec.Cmd {
	cmd := exec.Command("apt-get", args...)
	cmd.Env = aptEnv()
	return cmd
}

func configurePendingDebs() {
	cmd := exec.Command("dpkg", "--force-confdef", "--force-confold", "--configure", "-a")
	cmd.Env = aptEnv()
	_, _ = combinedTimeout(cmd, 4*time.Minute)
}

func aptUpdate() error {
	ensureDiskTmp()
	repairSirocRepos()
	cmd := aptCmd(append(aptOpts(), "update")...)
	out, err := combinedTimeout(cmd, 3*time.Minute)
	if err == nil {
		return nil
	}
	_ = os.RemoveAll("/var/lib/apt/lists/partial")
	clean := aptCmd("clean")
	_, _ = combinedTimeout(clean, 30*time.Second)
	cmd = aptCmd(append(aptOpts(), "update")...)
	out2, err2 := combinedTimeout(cmd, 3*time.Minute)
	if err2 != nil {
		return fmt.Errorf("apt-get update: %s: %w", tail(out+"\n"+out2), err2)
	}
	return nil
}

func AptInstall(pkgs ...string) error {
	return aptInstall(pkgs...)
}

func aptInstall(pkgs ...string) error {
	configurePendingDebs()
	cmd := aptCmd(append(aptOpts("install"), pkgs...)...)
	if out, err := combinedTimeout(cmd, 12*time.Minute); err != nil {
		configurePendingDebs()
		fix := aptCmd(append(aptOpts("-f", "install"), pkgs...)...)
		if out2, err2 := combinedTimeout(fix, 12*time.Minute); err2 != nil {
			return fmt.Errorf("apt-get install: %s: %w", tail(out+"\n"+out2), err)
		}
	}
	return nil
}

func aptRemove(pkgs ...string) {
	cmd := aptCmd(append(aptOpts("remove"), pkgs...)...)
	_, _ = combinedTimeout(cmd, 4*time.Minute)
}

func writeKeyring(url, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(ensureDiskTmp(), "siroc-key-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpName)
	dl := exec.Command("curl", "-fsSL", "-o", tmpName, url)
	if out, err := combinedTimeout(dl, 45*time.Second); err != nil {
		return fmt.Errorf("download key: %s: %w", tail(out), err)
	}
	_ = os.Remove(dest)
	dearmor := exec.Command("gpg", "--batch", "--yes", "--dearmor", "-o", dest, tmpName)
	if out, err := dearmor.CombinedOutput(); err != nil {
		if copyErr := copyFile(tmpName, dest); copyErr != nil {
			return fmt.Errorf("import key: %s: %w", strings.TrimSpace(string(out)), err)
		}
	}
	return os.Chmod(dest, 0644)
}

func writeRepo(name, line string) error {
	return os.WriteFile("/etc/apt/sources.list.d/cp-"+name+".list", []byte(strings.TrimSpace(line)+"\n"), 0644)
}

func removeRepo(name string) {
	_ = os.Remove("/etc/apt/sources.list.d/cp-" + name + ".list")
}

func nginxVersions() []string {
	return []string{"1.24", "1.30", "1.31"}
}

func apacheVersions() []string {
	return []string{"2.4"}
}

func mysqlVersions() []string {
	return []string{"8.0", "8.4", "9.7"}
}

func mariadbVersions() []string {
	return []string{"10.11", "11.4", "11.8"}
}

func pickVersion(have []string, version, fallback string) (string, error) {
	if version == "" {
		version = fallback
	}
	for _, v := range have {
		if v == version {
			return version, nil
		}
	}
	return "", fmt.Errorf("unsupported version %s", version)
}

func installNginx(version string) error {
	version, err := pickVersion(nginxVersions(), version, "1.30")
	if err != nil {
		return err
	}
	id, code := osRelease()
	switch version {
	case "1.24":
		removeRepo("nginx")
	case "1.30":
		if err := writeVendorRepo("nginx", "https://nginx.org/keys/nginx_signing.key", fmt.Sprintf("https://nginx.org/packages/%s", id), id, code, "nginx"); err != nil {
			return err
		}
	case "1.31":
		if err := writeVendorRepo("nginx", "https://nginx.org/keys/nginx_signing.key", fmt.Sprintf("https://nginx.org/packages/mainline/%s", id), id, code, "nginx"); err != nil {
			return err
		}
	}
	bak, err := backupNginxSites()
	if err != nil {
		return err
	}
	if err := aptUpdate(); err != nil {
		return err
	}
	aptRemove("nginx", "nginx-common", "nginx-core", "nginx-full")
	if err := aptInstall("nginx"); err != nil {
		restoreNginxSites(bak)
		return err
	}
	restoreNginxSites(bak)
	return configureNginxFrontend(version)
}

func backupNginxSites() (string, error) {
	src := "/etc/nginx/sites-available"
	if _, err := os.Stat(src); err != nil {
		return "", nil
	}
	bak := "/var/lib/siroc/nginx-sites-bak"
	_ = os.RemoveAll(bak)
	if err := os.MkdirAll("/var/lib/siroc", 0750); err != nil {
		return "", err
	}
	if out, err := exec.Command("cp", "-a", src, bak).CombinedOutput(); err != nil {
		return "", fmt.Errorf("backup nginx sites: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return bak, nil
}

func restoreNginxSites(bak string) {
	if bak == "" {
		return
	}
	if _, err := os.Stat(bak); err != nil {
		return
	}
	_ = os.MkdirAll("/etc/nginx/sites-available", 0755)
	_ = os.MkdirAll("/etc/nginx/sites-enabled", 0755)
	_ = exec.Command("cp", "-a", bak+"/.", "/etc/nginx/sites-available/").Run()
	ents, _ := os.ReadDir("/etc/nginx/sites-available")
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".conf") {
			continue
		}
		link := filepath.Join("/etc/nginx/sites-enabled", e.Name())
		target := filepath.Join("/etc/nginx/sites-available", e.Name())
		_ = os.Remove(link)
		_ = os.Symlink(target, link)
	}
}

func ensureNginxSitesLayout() error {
	for _, d := range []string{"/etc/nginx/sites-available", "/etc/nginx/sites-enabled", "/etc/nginx/conf.d"} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	_ = os.Remove("/etc/nginx/conf.d/default.conf")
	conf, _ := os.ReadFile("/etc/nginx/nginx.conf")
	if strings.Contains(string(conf), "sites-enabled") {
		_ = os.Remove("/etc/nginx/conf.d/cp-sites.conf")
		return nil
	}
	return os.WriteFile("/etc/nginx/conf.d/cp-sites.conf", []byte("include /etc/nginx/sites-enabled/*;\n"), 0644)
}

func installMySQL(version string) error {
	version, err := pickVersion(mysqlVersions(), version, "8.4")
	if err != nil {
		return err
	}
	removeRepo("mariadb")
	id, code := osRelease()
	comp := map[string]string{
		"8.0": "mysql-8.0",
		"8.4": "mysql-8.4-lts",
		"9.7": "mysql-9.7-lts",
	}[version]
	mirror := fmt.Sprintf("http://repo.mysql.com/apt/%s", id)
	suite := firstWorkingSuite(mirror, id, code)
	if suite == "" {
		return fmt.Errorf("MySQL has no apt repo for %s %s", id, code)
	}
	if err := writeKeyring("https://repo.mysql.com/RPM-GPG-KEY-mysql-2023", "/etc/apt/keyrings/cp-mysql.gpg"); err != nil {
		return err
	}
	line := fmt.Sprintf("deb [signed-by=/etc/apt/keyrings/cp-mysql.gpg] %s %s %s mysql-tools", mirror, suite, comp)
	if err := writeRepo("mysql", line); err != nil {
		return err
	}
	if err := aptUpdate(); err != nil {
		return err
	}
	for _, sel := range []string{
		"mysql-community-server mysql-community-server/root-pass password ",
		"mysql-community-server mysql-community-server/re-root-pass password ",
		"mysql-community-server mysql-server/default-auth-override select Use Strong Password Encryption (RECOMMENDED)",
	} {
		cmd := exec.Command("debconf-set-selections")
		cmd.Stdin = strings.NewReader(sel + "\n")
		_ = cmd.Run()
	}
	if err := aptInstall("mysql-community-server"); err != nil {
		return err
	}
	_ = exec.Command("mysql", "--batch", "-e", "ALTER USER 'root'@'localhost' IDENTIFIED WITH auth_socket; FLUSH PRIVILEGES;").Run()
	_ = exec.Command("systemctl", "enable", "--now", "mysql").Run()
	return nil
}

func installMariaDB(version string) error {
	version, err := pickVersion(mariadbVersions(), version, "11.8")
	if err != nil {
		return err
	}
	removeRepo("mysql")
	id, code := osRelease()
	mirror := fmt.Sprintf("https://deb.mariadb.org/%s/%s", version, id)
	suite := firstWorkingSuite(mirror, id, code)
	if suite != "" {
		if err := writeKeyring("https://mariadb.org/mariadb_release_signing_key.pgp", "/etc/apt/keyrings/cp-mariadb.gpg"); err != nil {
			return err
		}
		line := fmt.Sprintf("deb [signed-by=/etc/apt/keyrings/cp-mariadb.gpg] %s %s main", mirror, suite)
		if err := writeRepo("mariadb", line); err != nil {
			return err
		}
	} else {
		removeRepo("mariadb")
	}
	if err := aptUpdate(); err != nil {
		return err
	}
	if err := aptInstall("mariadb-server"); err != nil {
		return err
	}
	_ = exec.Command("systemctl", "enable", "--now", "mariadb").Run()
	return nil
}

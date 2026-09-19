//go:build linux

package software

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/validate"
)

type phpExtSpec struct {
	Name     string
	Title    string
	AptPkg   string
	Pecl     string
	Deps     []string
	Zend     bool
	Provides []string
}

func phpExtCatalog() []phpExtSpec {
	return []phpExtSpec{
		{Name: "opcache", Title: "OPcache", AptPkg: "opcache", Zend: true, Provides: []string{"opcache"}},
		{Name: "mysql", Title: "MySQL / mysqli", AptPkg: "mysql", Provides: []string{"mysqli", "pdo_mysql", "mysqlnd"}},
		{Name: "xml", Title: "XML / DOM", AptPkg: "xml", Provides: []string{"xml", "dom", "simplexml", "xmlreader", "xmlwriter"}},
		{Name: "curl", Title: "cURL", AptPkg: "curl"},
		{Name: "mbstring", Title: "Multibyte String", AptPkg: "mbstring"},
		{Name: "zip", Title: "Zip", AptPkg: "zip"},
		{Name: "gd", Title: "GD images", AptPkg: "gd"},
		{Name: "intl", Title: "Intl", AptPkg: "intl"},
		{Name: "bcmath", Title: "BCMath", AptPkg: "bcmath"},
		{Name: "soap", Title: "SOAP", AptPkg: "soap"},
		{Name: "sqlite3", Title: "SQLite3", AptPkg: "sqlite3"},
		{Name: "pgsql", Title: "PostgreSQL", AptPkg: "pgsql"},
		{Name: "ldap", Title: "LDAP", AptPkg: "ldap"},
		{Name: "imap", Title: "IMAP", AptPkg: "imap"},
		{Name: "gmp", Title: "GMP", AptPkg: "gmp"},
		{Name: "bz2", Title: "Bzip2", AptPkg: "bz2"},
		{Name: "xsl", Title: "XSL", AptPkg: "xsl"},
		{Name: "tidy", Title: "Tidy", AptPkg: "tidy"},
		{Name: "snmp", Title: "SNMP", AptPkg: "snmp"},
		{Name: "odbc", Title: "ODBC", AptPkg: "odbc"},
		{Name: "enchant", Title: "Enchant", AptPkg: "enchant"},
		{Name: "pspell", Title: "PSpell", AptPkg: "pspell"},
		{Name: "readline", Title: "Readline", AptPkg: "readline"},
		{Name: "redis", Title: "Redis", AptPkg: "redis", Pecl: "redis"},
		{Name: "memcached", Title: "Memcached", AptPkg: "memcached", Pecl: "memcached", Deps: []string{"libmemcached-dev", "zlib1g-dev"}},
		{Name: "apcu", Title: "APCu", AptPkg: "apcu", Pecl: "apcu"},
		{Name: "imagick", Title: "ImageMagick", AptPkg: "imagick", Pecl: "imagick", Deps: []string{"libmagickwand-dev"}},
		{Name: "xdebug", Title: "Xdebug", AptPkg: "xdebug", Pecl: "xdebug", Zend: true},
		{Name: "yaml", Title: "YAML", AptPkg: "yaml", Pecl: "yaml", Deps: []string{"libyaml-dev"}},
		{Name: "ssh2", Title: "SSH2", AptPkg: "ssh2", Pecl: "ssh2", Deps: []string{"libssh2-1-dev"}},
		{Name: "mongodb", Title: "MongoDB", AptPkg: "mongodb", Pecl: "mongodb", Deps: []string{"pkg-config", "libssl-dev"}},
		{Name: "igbinary", Title: "igbinary", AptPkg: "igbinary", Pecl: "igbinary"},
		{Name: "msgpack", Title: "msgpack", AptPkg: "msgpack", Pecl: "msgpack"},
		{Name: "mcrypt", Title: "Mcrypt", AptPkg: "mcrypt", Pecl: "mcrypt", Deps: []string{"libmcrypt-dev"}},
		{Name: "mailparse", Title: "Mailparse", AptPkg: "mailparse", Pecl: "mailparse"},
		{Name: "uuid", Title: "UUID", AptPkg: "uuid", Pecl: "uuid", Deps: []string{"uuid-dev"}},
		{Name: "maxminddb", Title: "MaxMind DB", AptPkg: "maxminddb"},
		{Name: "swoole", Title: "Swoole", Pecl: "swoole", Deps: []string{"libcurl4-openssl-dev", "libssl-dev"}},
		{Name: "ds", Title: "Data Structures", Pecl: "ds"},
		{Name: "excimer", Title: "Excimer", Pecl: "excimer"},
		{Name: "uploadprogress", Title: "Upload progress", Pecl: "uploadprogress"},
	}
}

func lookupPHPExt(name string) phpExtSpec {
	name = strings.ToLower(name)
	for _, e := range phpExtCatalog() {
		if e.Name == name {
			return e
		}
		for _, p := range e.Provides {
			if p == name {
				return e
			}
		}
	}
	return phpExtSpec{Name: name, Title: name, Pecl: name, Provides: []string{name}}
}

func phpExtMods(e phpExtSpec) []string {
	if len(e.Provides) > 0 {
		return e.Provides
	}
	return []string{e.Name}
}

func (m *Manager) ListPHPExt(version string) rpc.PHPExtListResp {
	if version == "" {
		version = "8.3"
	}
	seen := map[string]struct{}{}
	var out []rpc.PHPExtInfo
	aptNames := phpAptPkgnames(version)
	for _, e := range phpExtCatalog() {
		seen[e.Name] = struct{}{}
		src := "apt"
		if e.AptPkg != "" && e.Pecl != "" {
			src = "both"
		} else if e.Pecl != "" && e.AptPkg == "" {
			src = "pecl"
		}
		info := rpc.PHPExtInfo{
			Name:      e.Name,
			Title:     e.Title,
			Source:    src,
			AptPkg:    e.AptPkg,
			Pecl:      e.Pecl,
			Zend:      e.Zend,
			Installed: phpExtInstalled(version, e),
			Available: e.Pecl != "" || aptNames["php"+version+"-"+e.AptPkg] || phpExtInstalled(version, e),
		}
		if e.AptPkg != "" && aptNames["php"+version+"-"+e.AptPkg] {
			info.Available = true
		}
		if info.Installed {
			info.Available = true
		}
		out = append(out, info)
	}
	mods := filepath.Join("/etc/php", version, "mods-available")
	entries, _ := os.ReadDir(mods)
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".ini") {
			continue
		}
		name := strings.TrimSuffix(ent.Name(), ".ini")
		if _, ok := seen[name]; ok {
			continue
		}
		owned := false
		for _, e := range phpExtCatalog() {
			for _, p := range phpExtMods(e) {
				if p == name {
					owned = true
					break
				}
			}
			if owned {
				break
			}
		}
		if owned {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, rpc.PHPExtInfo{
			Name:      name,
			Title:     name,
			Source:    "system",
			Installed: true,
			Available: true,
			Zend:      phpIniIsZend(filepath.Join(mods, ent.Name())),
		})
	}
	return rpc.PHPExtListResp{Version: version, Extensions: out}
}

func phpExtInstalled(ver string, e phpExtSpec) bool {
	if e.AptPkg != "" {
		if ok, _ := dpkgVersion("php" + ver + "-" + e.AptPkg); ok {
			return true
		}
	}
	for _, mod := range phpExtMods(e) {
		if _, err := os.Stat(filepath.Join("/etc/php", ver, "mods-available", mod+".ini")); err == nil {
			return true
		}
	}
	return false
}

func phpAptPkgnames(ver string) map[string]bool {
	out := map[string]bool{}
	b, err := exec.Command("apt-cache", "pkgnames", "php"+ver+"-").Output()
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out[line] = true
		}
	}
	return out
}

func phpIniIsZend(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(b), "zend_extension")
}

func (m *Manager) InstallPHPExt(spec string) error {
	ver, name, source, err := rpc.ParsePHPExtJob(spec)
	if err != nil {
		return err
	}
	if err := validate.PHPVersion(ver); err != nil {
		return err
	}
	if err := validate.PHPExtName(name); err != nil {
		return err
	}
	item := lookupPHPExt(name)
	if phpExtInstalled(ver, item) {
		_ = enablePHPExt(ver, item)
		_ = exec.Command("systemctl", "reload", "php"+ver+"-fpm").Run()
		return nil
	}
	switch source {
	case "apt":
		return installPHPExtApt(ver, item)
	case "pecl":
		return installPHPExtPecl(ver, item)
	default:
		if item.AptPkg != "" {
			if err := installPHPExtApt(ver, item); err == nil {
				return nil
			}
		}
		if item.Pecl == "" && item.Name != "" {
			item.Pecl = item.Name
		}
		return installPHPExtPecl(ver, item)
	}
}

func installPHPExtApt(ver string, item phpExtSpec) error {
	if item.AptPkg == "" {
		return fmt.Errorf("%s has no apt package for PHP %s", item.Name, ver)
	}
	if err := ensureRepos("php"); err != nil {
		return err
	}
	pkg := "php" + ver + "-" + item.AptPkg
	if err := aptInstall(pkg); err != nil {
		return fmt.Errorf("apt-get install %s: %w", pkg, err)
	}
	if err := enablePHPExt(ver, item); err != nil {
		return err
	}
	_ = exec.Command("systemctl", "reload", "php"+ver+"-fpm").Run()
	return nil
}

func installPHPExtPecl(ver string, item phpExtSpec) error {
	peclName := item.Pecl
	if peclName == "" {
		peclName = item.Name
	}
	if err := ensureRepos("php"); err != nil {
		return err
	}
	pkgs := []string{"php" + ver + "-dev", "php-pear", "build-essential", "pkg-config"}
	pkgs = append(pkgs, item.Deps...)
	if err := aptInstall(pkgs...); err != nil {
		return fmt.Errorf("install pecl build deps: %w", err)
	}
	phpize := "/usr/bin/phpize" + ver
	phpconfig := "/usr/bin/php-config" + ver
	if _, err := os.Stat(phpize); err != nil {
		return fmt.Errorf("phpize%s not found; install PHP %s first", ver, ver)
	}
	tmp, err := os.MkdirTemp("", "siroc-pecl-"+item.Name+"-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	dl := exec.Command("pecl", "download", peclName)
	dl.Dir = tmp
	dl.Env = append(os.Environ(), "PHP_PEAR_PHP_BIN=/usr/bin/php"+ver)
	if out, err := combinedTimeout(dl, 3*time.Minute); err != nil {
		return fmt.Errorf("pecl download %s: %s: %w", peclName, tail(out), err)
	}
	tgz := ""
	entries, _ := os.ReadDir(tmp)
	for _, e := range entries {
		n := e.Name()
		if strings.HasSuffix(n, ".tgz") || strings.HasSuffix(n, ".tar.gz") {
			tgz = filepath.Join(tmp, n)
			break
		}
	}
	if tgz == "" {
		return fmt.Errorf("pecl download %s did not produce an archive", peclName)
	}
	untar := exec.Command("tar", "xzf", tgz, "-C", tmp)
	if out, err := combinedTimeout(untar, 1*time.Minute); err != nil {
		return fmt.Errorf("extract pecl %s: %s: %w", peclName, tail(out), err)
	}
	src := ""
	entries, _ = os.ReadDir(tmp)
	for _, e := range entries {
		if e.IsDir() {
			src = filepath.Join(tmp, e.Name())
			break
		}
	}
	if src == "" {
		return fmt.Errorf("pecl archive %s had no source directory", peclName)
	}
	steps := []struct {
		name string
		cmd  *exec.Cmd
		wait time.Duration
	}{
		{"phpize", exec.Command(phpize), 2 * time.Minute},
		{"configure", exec.Command("./configure", "--with-php-config="+phpconfig), 3 * time.Minute},
		{"make", exec.Command("make", "-j2"), 10 * time.Minute},
		{"make install", exec.Command("make", "install"), 2 * time.Minute},
	}
	for _, st := range steps {
		st.cmd.Dir = src
		st.cmd.Env = append(os.Environ(), "PHP_PEAR_PHP_BIN=/usr/bin/php"+ver)
		if out, err := combinedTimeout(st.cmd, st.wait); err != nil {
			return fmt.Errorf("pecl %s %s: %s: %w", peclName, st.name, tail(out), err)
		}
	}
	line := "extension=" + item.Name + ".so\n"
	if item.Zend {
		line = "zend_extension=" + item.Name + ".so\n"
	}
	ini := filepath.Join("/etc/php", ver, "mods-available", item.Name+".ini")
	if err := os.MkdirAll(filepath.Dir(ini), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(ini, []byte(line), 0644); err != nil {
		return err
	}
	if err := enablePHPExt(ver, item); err != nil {
		return err
	}
	_ = exec.Command("systemctl", "reload", "php"+ver+"-fpm").Run()
	return nil
}

func enablePHPExt(ver string, item phpExtSpec) error {
	var last error
	for _, mod := range phpExtMods(item) {
		ini := filepath.Join("/etc/php", ver, "mods-available", mod+".ini")
		if _, err := os.Stat(ini); err != nil {
			continue
		}
		cmd := exec.Command("phpenmod", "-v", ver, "-s", "ALL", mod)
		if out, err := cmd.CombinedOutput(); err != nil {
			last = fmt.Errorf("phpenmod %s: %s: %w", mod, strings.TrimSpace(string(out)), err)
		}
	}
	return last
}

func PHPExtIniFiles(ver string, names []string) []string {
	var files []string
	seen := map[string]struct{}{}
	for _, name := range names {
		item := lookupPHPExt(name)
		for _, mod := range phpExtMods(item) {
			if _, ok := seen[mod]; ok {
				continue
			}
			seen[mod] = struct{}{}
			p := filepath.Join("/etc/php", ver, "mods-available", mod+".ini")
			if _, err := os.Stat(p); err == nil {
				files = append(files, p)
			}
		}
		p := filepath.Join("/etc/php", ver, "mods-available", name+".ini")
		if _, err := os.Stat(p); err == nil {
			if _, ok := seen[name]; !ok {
				files = append(files, p)
			}
		}
	}
	return files
}

func CorePHPExtNames(ver string) []string {
	optional := map[string]struct{}{}
	for _, e := range phpExtCatalog() {
		optional[e.Name] = struct{}{}
		for _, p := range phpExtMods(e) {
			optional[p] = struct{}{}
		}
	}
	var names []string
	mods := filepath.Join("/etc/php", ver, "mods-available")
	entries, _ := os.ReadDir(mods)
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".ini") {
			continue
		}
		name := strings.TrimSuffix(ent.Name(), ".ini")
		if _, ok := optional[name]; ok {
			continue
		}
		names = append(names, name)
	}
	return names
}

func EnabledPHPExtNames(ver string) []string {
	var names []string
	seen := map[string]struct{}{}
	for _, e := range phpExtCatalog() {
		if phpExtInstalled(ver, e) {
			names = append(names, e.Name)
			seen[e.Name] = struct{}{}
		}
	}
	mods := filepath.Join("/etc/php", ver, "mods-available")
	entries, _ := os.ReadDir(mods)
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".ini") {
			continue
		}
		name := strings.TrimSuffix(ent.Name(), ".ini")
		if _, ok := seen[name]; ok {
			continue
		}
		owned := false
		for _, e := range phpExtCatalog() {
			for _, p := range phpExtMods(e) {
				if p == name {
					owned = true
					break
				}
			}
		}
		if owned {
			continue
		}
		names = append(names, name)
		seen[name] = struct{}{}
	}
	return names
}

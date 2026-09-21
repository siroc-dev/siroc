//go:build linux

package software

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/security"
	"github.com/siroc-dev/siroc/internal/weblog"
)

type Manager struct{}

type spec struct {
	Name        string
	Title       string
	Service     string
	AptPkg      string
	ExclusiveOf string
	Versions    func() []string
	Installed   func() (bool, string)
	PostInstall func(version string) error
	InstallPkg  func(version string) string
}

func catalog() []spec {
	return []spec{
		{
			Name:    "nginx",
			Title:   "Nginx",
			Service: "nginx",
			AptPkg:  "nginx",
			Versions: func() []string {
				return nginxVersions()
			},
			Installed: func() (bool, string) {
				return dpkgVersion("nginx")
			},
			PostInstall: configureNginxFrontend,
		},
		{
			Name:    "apache",
			Title:   "Apache",
			Service: "apache2",
			AptPkg:  "apache2",
			Versions: func() []string {
				return apacheVersions()
			},
			Installed: func() (bool, string) {
				return dpkgVersion("apache2")
			},
			PostInstall: configureApacheBackend,
		},
		{
			Name:    "php",
			Title:   "PHP-FPM",
			Service: "",
			Versions: func() []string {
				return []string{"8.1", "8.2", "8.3", "8.4"}
			},
			InstallPkg: func(v string) string {
				if v == "" {
					v = BasePHPVersion
				}
				return fmt.Sprintf("php%s-fpm php%s-cli php%s-mysql php%s-xml php%s-curl php%s-mbstring php%s-zip php%s-gd", v, v, v, v, v, v, v, v)
			},
			Installed: func() (bool, string) {
				found := phpInstalledVersions()
				if len(found) == 0 {
					return false, ""
				}
				return true, strings.Join(found, ", ")
			},
			PostInstall: func(version string) error {
				if version == "" {
					version = BasePHPVersion
				}
				_ = exec.Command("systemctl", "enable", "--now", "php"+version+"-fpm").Run()
				_ = setPHPCLI(version)
				return enableApachePHP()
			},
		},
		{
			Name:        "mysql",
			Title:       "MySQL",
			Service:     "mysql",
			AptPkg:      "mysql-server",
			ExclusiveOf: "mariadb",
			Versions: func() []string {
				return mysqlVersions()
			},
			Installed: func() (bool, string) {
				if ok, ver := sqlSourceInstalled("mysql"); ok {
					return true, ver
				}
				if ok, ver := dpkgVersion("mysql-server"); ok {
					return true, ver
				}
				return dpkgVersion("mysql-community-server")
			},
		},
		{
			Name:        "mariadb",
			Title:       "MariaDB",
			Service:     "mariadb",
			AptPkg:      "mariadb-server",
			ExclusiveOf: "mysql",
			Versions: func() []string {
				return mariadbVersions()
			},
			Installed: func() (bool, string) {
				if ok, ver := sqlSourceInstalled("mariadb"); ok {
					return true, ver
				}
				return dpkgVersion("mariadb-server")
			},
		},
		{
			Name:    "redis",
			Title:   "Redis",
			Service: "redis-server",
			AptPkg:  "redis-server",
			Versions: func() []string {
				return redisVersions()
			},
			Installed: func() (bool, string) {
				return redisInstalled()
			},
		},
		{
			Name:    "python",
			Title:   "Python",
			Service: "",
			Versions: func() []string {
				return []string{"3.10", "3.11", "3.12", "3.13"}
			},
			InstallPkg: func(v string) string {
				if v == "" {
					v = "3.12"
				}
				return fmt.Sprintf("python%s python%s-venv python%s-dev", v, v, v)
			},
			Installed: func() (bool, string) {
				found := pythonInstalledVersions()
				if len(found) == 0 {
					if ok, ver := dpkgVersion("python3"); ok {
						return true, ver
					}
					return false, ""
				}
				return true, strings.Join(found, ", ")
			},
			PostInstall: func(version string) error {
				if version == "" {
					version = "3.12"
				}
				return setPythonCLI(version)
			},
		},
		{
			Name:    "nodejs",
			Title:   "Node.js",
			Service: "",
			Versions: func() []string {
				return []string{"18", "20", "22"}
			},
			Installed: func() (bool, string) {
				found := nodeInstalledVersions()
				if len(found) == 0 {
					return false, ""
				}
				cli := currentNodeCLI()
				if cli != "" {
					return true, "CLI " + cli + " (" + strings.Join(found, ", ") + ")"
				}
				return true, strings.Join(found, ", ")
			},
		},
		{
			Name:   "golang",
			Title:  "Go",
			AptPkg: "golang-go",
			Installed: func() (bool, string) {
				return binVersion("go")
			},
		},
		{
			Name:   "rust",
			Title:  "Rust",
			AptPkg: "rustc cargo",
			Installed: func() (bool, string) {
				ok, ver := binVersion("rustc")
				if !ok {
					return false, ""
				}
				if _, cargo := binVersion("cargo"); cargo != "" {
					return true, ver
				}
				return true, ver
			},
		},
		{
			Name:    "docker",
			Title:   "Docker",
			Service: "docker",
			AptPkg:  "docker.io docker-compose-v2",
			Installed: func() (bool, string) {
				return binVersion("docker")
			},
			PostInstall: func(string) error {
				_ = exec.Command("systemctl", "enable", "--now", "docker").Run()
				return nil
			},
		},
		{
			Name:    "certbot",
			Title:   "Let's Encrypt (Certbot)",
			Service: "",
			AptPkg:  "certbot",
			Installed: func() (bool, string) {
				return dpkgVersion("certbot")
			},
			PostInstall: configureCertbot,
		},
		{
			Name:    "ufw",
			Title:   "UFW firewall",
			Service: "",
			AptPkg:  "ufw",
			Installed: func() (bool, string) {
				return dpkgVersion("ufw")
			},
			PostInstall: func(string) error {
				security.ApplyDefaultUFW()
				return nil
			},
		},
		{
			Name:    "waf",
			Title:   "ModSecurity WAF",
			Service: "",
			AptPkg:  "libapache2-mod-security2 modsecurity-crs",
			Installed: func() (bool, string) {
				return dpkgVersion("libapache2-mod-security2")
			},
			PostInstall: configureWAF,
		},
		{
			Name:    "clamav",
			Title:   "ClamAV antivirus",
			Service: "clamav-daemon",
			AptPkg:  "clamav clamav-daemon clamav-freshclam",
			Installed: func() (bool, string) {
				return dpkgVersion("clamav")
			},
			PostInstall: configureClamAV,
		},
		{
			Name:    "openssh",
			Title:   "OpenSSH server",
			Service: "ssh",
			AptPkg:  "openssh-server",
			Installed: func() (bool, string) {
				return dpkgVersion("openssh-server")
			},
			PostInstall: configureSSH,
		},
		{
			Name:    "vsftpd",
			Title:   "FTP (vsftpd)",
			Service: "vsftpd",
			AptPkg:  "vsftpd",
			Installed: func() (bool, string) {
				return dpkgVersion("vsftpd")
			},
			PostInstall: configureVSFTPD,
		},
		{
			Name:    "goaccess",
			Title:   "GoAccess (web stats)",
			Service: "",
			AptPkg:  "goaccess",
			Installed: func() (bool, string) {
				return dpkgVersion("goaccess")
			},
			PostInstall: func(string) error {
				return weblog.GenerateAll()
			},
		},
		{
			Name:    "supervisor",
			Title:   "Supervisord (Laravel queue)",
			Service: "supervisor",
			AptPkg:  "supervisor",
			Installed: func() (bool, string) {
				return dpkgVersion("supervisor")
			},
			PostInstall: func(string) error {
				_ = exec.Command("systemctl", "enable", "--now", "supervisor").Run()
				_ = exec.Command("systemctl", "enable", "--now", "supervisord").Run()
				return nil
			},
		},
		{
			Name:  "composer",
			Title: "Composer",
			Installed: func() (bool, string) {
				return binVersion("composer")
			},
		},
		{
			Name:  "wp-cli",
			Title: "WP-CLI",
			Installed: func() (bool, string) {
				return binVersion("wp")
			},
		},
		{
			Name:   "nikto",
			Title:  "Nikto",
			AptPkg: "nikto",
			Installed: func() (bool, string) {
				return niktoInstalled()
			},
		},
		{
			Name:  "zap",
			Title: "OWASP ZAP",
			Installed: func() (bool, string) {
				return zapInstalled()
			},
		},
		{
			Name:    "openvas",
			Title:   "OpenVAS / Greenbone",
			Service: "gvmd",
			Installed: func() (bool, string) {
				return openvasInstalled()
			},
		},
		{
			Name:    "memcached",
			Title:   "Memcached",
			Service: "memcached",
			AptPkg:  "memcached",
			Installed: func() (bool, string) {
				if _, err := exec.LookPath("memcached"); err != nil {
					return false, ""
				}
				out, _ := exec.Command("memcached", "-h").CombinedOutput()
				ver := ""
				for _, line := range strings.Split(string(out), "\n") {
					if strings.Contains(strings.ToLower(line), "memcached") {
						ver = strings.TrimSpace(line)
						break
					}
				}
				return true, ver
			},
		},
		{
			Name:  "ffmpeg",
			Title: "FFmpeg",
			Versions: func() []string {
				return ffmpegVersions()
			},
			Installed: func() (bool, string) {
				found := ffmpegInstalledVersions()
				if len(found) == 0 {
					out, err := exec.Command("ffmpeg", "-version").CombinedOutput()
					if err != nil {
						return false, ""
					}
					return true, strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
				}
				cli := currentFFmpegCLI()
				label := strings.Join(found, ", ")
				if cli != "" {
					return true, "CLI " + cli + " (" + label + ")"
				}
				return true, label
			},
			PostInstall: func(version string) error {
				if version == "" {
					version = ffmpegDefaultVersion()
				}
				return setFFmpegCLI(version)
			},
		},
		{
			Name:    "fail2ban",
			Title:   "Fail2ban",
			Service: "fail2ban",
			AptPkg:  "fail2ban",
			PostInstall: func(string) error {
				_ = os.WriteFile("/etc/fail2ban/jail.local", []byte("[DEFAULT]\nbantime = 1h\nfindtime = 10m\nmaxretry = 5\n\n[sshd]\nenabled = true\n"), 0644)
				return exec.Command("systemctl", "enable", "--now", "fail2ban").Run()
			},
			Installed: func() (bool, string) {
				if _, err := exec.LookPath("fail2ban-client"); err != nil {
					return false, ""
				}
				out, _ := exec.Command("fail2ban-client", "-V").CombinedOutput()
				return true, strings.TrimSpace(string(out))
			},
		},
		{
			Name:  "phpmyadmin",
			Title: "phpMyAdmin",
			Installed: func() (bool, string) {
				return pmaInstalled()
			},
			PostInstall: func(string) error {
				return pmaSetup()
			},
		},
		{
			Name:   "quota",
			Title:  "Disk quotas",
			AptPkg: "quota",
			Installed: func() (bool, string) {
				if _, err := exec.LookPath("setquota"); err != nil {
					return false, ""
				}
				return true, "setquota"
			},
		},
	}
}

func (m *Manager) List() []rpc.PackageInfo {
	var out []rpc.PackageInfo
	for _, s := range catalog() {
		inst, ver := s.Installed()
		info := rpc.PackageInfo{
			Name:        s.Name,
			Title:       s.Title,
			Description: Description(s.Name),
			Installed:   inst,
			Version:     ver,
			Service:     s.Service,
			ExclusiveOf: s.ExclusiveOf,
		}
		if s.Versions != nil {
			info.Versions = s.Versions()
		}
		if s.Service != "" && inst {
			info.Active = serviceActive(s.Service)
		}
		if s.Name == "php" && inst {
			info.Active = phpAnyActive()
			info.Service = "php-fpm"
		}
		if s.Name == "ufw" && inst {
			info.Active = ufwActive()
			info.Service = "ufw"
		}
		if s.Name == "waf" && inst {
			info.Active = wafEnabled()
			info.Service = "waf"
		}
		if s.Name == "openvas" && inst {
			info.Active = serviceActive("gvmd") || serviceActive("gsad")
		}
		switch s.Name {
		case "php":
			info.InstalledVersions = phpInstalledVersions()
			info.CLIVersion = currentPHPCLI()
		case "python":
			info.InstalledVersions = pythonInstalledVersions()
			info.CLIVersion = currentPythonCLI()
		case "nodejs":
			info.InstalledVersions = nodeInstalledVersions()
			info.CLIVersion = currentNodeCLI()
		case "ffmpeg":
			info.InstalledVersions = ffmpegInstalledVersions()
			info.CLIVersion = currentFFmpegCLI()
		}
		out = append(out, info)
	}
	return out
}

func PrepareRuntime() {
	ensureDiskTmp()
	security.PinGvmTemp()
	RepairNodeRuntimes()
}

func (m *Manager) PHPInstalled() []string {
	var found []string
	for _, v := range []string{"8.1", "8.2", "8.3", "8.4"} {
		if ok, _ := dpkgVersion("php" + v + "-fpm"); ok {
			found = append(found, v)
		}
	}
	if found == nil {
		found = []string{}
	}
	return found
}

func (m *Manager) Install(name, version string) error {
	if up, ver := splitUpgrade(version); up {
		return m.upgrade(name, ver)
	}
	return m.install(name, version)
}

func splitUpgrade(version string) (bool, string) {
	v := strings.TrimSpace(version)
	if v == "upgrade" {
		return true, ""
	}
	if strings.HasPrefix(v, "upgrade:") {
		return true, strings.TrimPrefix(v, "upgrade:")
	}
	return false, version
}

func (m *Manager) install(name, version string) error {
	if name == "php-ext" {
		return m.InstallPHPExt(version)
	}
	var s *spec
	for i := range catalog() {
		item := catalog()[i]
		if item.Name == name {
			s = &item
			break
		}
	}
	if s == nil {
		return fmt.Errorf("unknown package %q", name)
	}
	if s.ExclusiveOf != "" {
		for _, other := range catalog() {
			if other.Name == s.ExclusiveOf {
				if ok, _ := other.Installed(); ok {
					return fmt.Errorf("%s is already installed; uninstall it before installing %s", other.Title, s.Title)
				}
			}
		}
	}
	if s.Versions != nil {
		allowed := s.Versions()
		fallback := ""
		if len(allowed) > 0 {
			fallback = allowed[len(allowed)-1]
		}
		if s.Name == "php" {
			fallback = BasePHPVersion
		}
		if s.Name == "ffmpeg" {
			fallback = ffmpegDefaultVersion()
		}
		v, err := pickVersion(allowed, version, fallback)
		if err != nil {
			return err
		}
		version = v
	}
	if name == "nodejs" {
		return installNode(version)
	}
	if name == "ffmpeg" {
		return installFFmpeg(version)
	}
	if name == "nginx" {
		return installNginx(version)
	}
	if name == "mysql" {
		return installMySQL(version)
	}
	if name == "mariadb" {
		return installMariaDB(version)
	}
	if name == "composer" {
		return installComposer()
	}
	if name == "wp-cli" {
		return installWPCLI()
	}
	if name == "redis" {
		return installRedis(version)
	}
	if name == "nikto" {
		return installNikto()
	}
	if name == "zap" {
		return installZAP()
	}
	if name == "openvas" {
		return installOpenVAS()
	}
	if name == "phpmyadmin" {
		return pmaSetup()
	}
	if err := ensureRepos(name); err != nil {
		return err
	}
	pkg := s.AptPkg
	if s.InstallPkg != nil {
		pkg = s.InstallPkg(version)
	}
	if err := aptInstall(strings.Fields(pkg)...); err != nil {
		return err
	}
	if s.PostInstall != nil {
		if err := s.PostInstall(version); err != nil {
			return err
		}
	}
	if s.Service != "" {
		_ = exec.Command("systemctl", "enable", "--now", s.Service).Run()
	}
	return nil
}

func (m *Manager) upgrade(name, version string) error {
	_ = aptUpdate()
	switch name {
	case "ffmpeg":
		return upgradeFFmpeg(version)
	case "nodejs":
		if version == "" {
			version = currentNodeCLI()
		}
		if version == "" {
			version = "22"
		}
		_ = os.RemoveAll(filepath.Join(nodeRuntimeRoot, version))
		return installNode(version)
	case "php":
		vers := phpInstalledVersions()
		if version != "" {
			vers = []string{version}
		}
		if len(vers) == 0 {
			vers = []string{"8.3"}
		}
		for _, v := range vers {
			if err := m.install("php", v); err != nil {
				return err
			}
		}
		return nil
	case "python":
		vers := pythonInstalledVersions()
		if version != "" {
			vers = []string{version}
		}
		if len(vers) == 0 {
			return m.install("python", version)
		}
		for _, v := range vers {
			if err := m.install("python", v); err != nil {
				return err
			}
		}
		return nil
	case "phpmyadmin":
		return pmaRefresh()
	case "composer":
		_ = os.Remove("/usr/local/bin/composer")
		return installComposer()
	case "wp-cli":
		_ = os.Remove("/usr/local/bin/wp")
		return installWPCLI()
	default:
		return m.install(name, version)
	}
}

func (m *Manager) Service(name, action string) error {
	switch action {
	case "start", "stop", "restart", "reload":
	default:
		return fmt.Errorf("invalid service action")
	}
	svc := ""
	if name == "php" || name == "php-fpm" {
		for _, v := range m.PHPInstalled() {
			if err := exec.Command("systemctl", action, "php"+v+"-fpm").Run(); err != nil {
				return fmt.Errorf("php%s-fpm %s: %w", v, action, err)
			}
		}
		return nil
	}
	if name == "ufw" {
		return controlUFW(action)
	}
	if name == "waf" {
		return controlWAF(action)
	}
	if name == "clamav" {
		_ = exec.Command("systemctl", action, "clamav-freshclam").Run()
	}
	if name == "openvas" && (action == "start" || action == "restart") {
		return security.PrepareOpenVAS()
	}
	if name == "openvas" && action == "stop" {
		for _, svc := range []string{"gsad", "gvmd", "ospd-openvas"} {
			_ = exec.Command("systemctl", "stop", svc).Run()
		}
		return nil
	}
	for _, s := range catalog() {
		if s.Name == name {
			svc = s.Service
			break
		}
	}
	if svc == "" {
		return fmt.Errorf("unknown service")
	}
	out, err := exec.Command("systemctl", action, svc).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func ensureRepos(name string) error {
	if err := aptUpdate(); err != nil {
		return err
	}
	switch name {
	case "php":
		return ensurePHPRepo()
	case "python":
		return ensurePythonRepo()
	default:
		return nil
	}
}

func ensurePHPRepo() error {
	id, code := osRelease()
	if id == "ubuntu" {
		_ = exec.Command("add-apt-repository", "-y", "-n", "universe").Run()
	}
	suite := firstWorkingSuite(suryPHPMirror, id, code)
	if suite != "" {
		signedBy, err := installSuryKeyring()
		if err != nil {
			return err
		}
		line := fmt.Sprintf("deb [signed-by=%s] %s/ %s main", signedBy, strings.TrimRight(suryPHPMirror, "/"), suite)
		if err := writeRepo("sury-php", line); err != nil {
			return err
		}
		return aptUpdate()
	}
	if id == "ubuntu" && firstWorkingSuite(launchpadPPAMirror("ondrej", "php", id), id, code) != "" {
		return addLaunchpadPPA("ondrej", "php")
	}
	return nil
}

func ensurePythonRepo() error {
	id, code := osRelease()
	if id != "ubuntu" {
		return nil
	}
	_ = exec.Command("add-apt-repository", "-y", "-n", "universe").Run()
	if firstWorkingSuite(launchpadPPAMirror("deadsnakes", "ppa", id), id, code) == "" {
		return nil
	}
	return addLaunchpadPPA("deadsnakes", "ppa")
}

func addLaunchpadPPA(owner, name string) error {
	if _, err := os.Stat("/usr/bin/add-apt-repository"); err != nil {
		if err := aptInstall("software-properties-common", "ca-certificates", "gnupg", "apt-transport-https"); err != nil {
			return fmt.Errorf("install software-properties-common: %w", err)
		}
	}
	ppa := "ppa:" + owner + "/" + name
	add := exec.Command("add-apt-repository", "-y", "-n", ppa)
	add.Env = aptEnv()
	if _, err := os.Stat("/usr/bin/python3.12"); err == nil {
		add = exec.Command("/usr/bin/python3.12", "/usr/bin/add-apt-repository", "-y", "-n", ppa)
		add.Env = append(aptEnv(), "PYTHONDONTWRITEBYTECODE=1")
	}
	out, addErr := combinedTimeout(add, 4*time.Minute)
	if err := aptUpdate(); err != nil {
		if addErr != nil {
			return fmt.Errorf("add %s: %s: %w", ppa, tail(out), addErr)
		}
		return err
	}
	return nil
}

func configureCertbot(string) error {
	if err := os.MkdirAll("/var/www/letsencrypt/.well-known/acme-challenge", 0755); err != nil {
		return err
	}
	if err := os.MkdirAll("/etc/letsencrypt/renewal-hooks/deploy", 0755); err != nil {
		return err
	}
	hook := "#!/bin/sh\nsystemctl reload nginx >/dev/null 2>&1 || true\n"
	if err := os.WriteFile("/etc/letsencrypt/renewal-hooks/deploy/cp-reload-nginx.sh", []byte(hook), 0755); err != nil {
		return err
	}
	_ = exec.Command("systemctl", "enable", "--now", "certbot.timer").Run()
	return nil
}

func configureNginxFrontend(string) error {
	if err := ensureNginxSitesLayout(); err != nil {
		return err
	}
	_ = os.Remove("/etc/nginx/sites-enabled/default")
	_ = os.MkdirAll("/var/www/letsencrypt/.well-known/acme-challenge", 0755)
	stub := `server {
    listen 80 default_server;
    listen [::]:80 default_server;
    server_name _;
    location ^~ /.well-known/acme-challenge/ {
        root /var/www/letsencrypt;
        default_type text/plain;
    }
    location ^~ /server-status { return 404; }
    location ^~ /nginx-status { return 404; }
    location ^~ /fpm-status { return 404; }
    location / {
        return 444;
    }
}
`
	if err := os.WriteFile("/etc/nginx/sites-available/siroc-default.conf", []byte(stub), 0644); err != nil {
		return err
	}
	_ = os.Remove("/etc/nginx/sites-enabled/siroc-default.conf")
	if err := os.Symlink("/etc/nginx/sites-available/siroc-default.conf", "/etc/nginx/sites-enabled/siroc-default.conf"); err != nil && !os.IsExist(err) {
		return err
	}
	if err := exec.Command("systemctl", "enable", "--now", "nginx").Run(); err != nil {
		return err
	}
	_ = enableNginxStatus()
	return weblog.ApplyNginx()
}

func configureApacheBackend(string) error {
	ports := "Listen 8080\n"
	if err := os.WriteFile("/etc/apache2/ports.conf", []byte(ports), 0644); err != nil {
		return err
	}
	_ = exec.Command("a2dissite", "000-default").Run()
	_ = exec.Command("a2enmod", "proxy", "proxy_fcgi", "rewrite", "headers", "setenvif", "status").Run()
	_ = enableApacheStatus()
	conf := `<VirtualHost 127.0.0.1:8080>
    ServerName _default_
    DocumentRoot /var/www/html
    <Directory /var/www/html>
        AllowOverride All
        Require all granted
    </Directory>
</VirtualHost>
`
	if err := os.WriteFile("/etc/apache2/sites-available/siroc-backend.conf", []byte(conf), 0644); err != nil {
		return err
	}
	_ = exec.Command("a2ensite", "siroc-backend").Run()
	return exec.Command("systemctl", "restart", "apache2").Run()
}

func enableApachePHP() error {
	if _, err := exec.LookPath("a2enmod"); err != nil {
		return nil
	}
	return exec.Command("a2enmod", "proxy", "proxy_fcgi", "setenvif").Run()
}

func dpkgVersion(pkg string) (bool, string) {
	out, err := exec.Command("dpkg-query", "-W", "-f", "${Status}|${Version}", pkg).CombinedOutput()
	if err != nil {
		return false, ""
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), "|", 2)
	if len(parts) != 2 || !strings.Contains(parts[0], "install ok installed") {
		return false, ""
	}
	return true, parts[1]
}

func binVersion(name string) (bool, string) {
	p, err := exec.LookPath(name)
	if err != nil {
		return false, ""
	}
	out, _ := exec.Command(p, "--version").CombinedOutput()
	line := strings.TrimSpace(string(out))
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	if line == "" {
		line = name
	}
	return true, line
}

func installComposer() error {
	if ok, _ := binVersion("composer"); ok {
		return nil
	}
	home := "/var/lib/siroc/composer"
	_ = os.MkdirAll(home, 0750)
	setup := "/var/lib/siroc/composer-setup.php"
	if out, err := combinedTimeout(exec.Command("curl", "-fsSL", "-o", setup, "https://getcomposer.org/installer"), 2*time.Minute); err != nil {
		return fmt.Errorf("download composer: %s: %w", tail(out), err)
	}
	defer os.Remove(setup)
	cmd := exec.Command("php", setup, "--install-dir=/usr/local/bin", "--filename=composer")
	cmd.Env = append(aptEnv(), "HOME=/root", "COMPOSER_HOME="+home)
	if out, err := combinedTimeout(cmd, 2*time.Minute); err != nil {
		return fmt.Errorf("install composer: %s: %w", tail(out), err)
	}
	return os.Chmod("/usr/local/bin/composer", 0755)
}

func installWPCLI() error {
	if ok, _ := binVersion("wp"); ok {
		return nil
	}
	if out, err := combinedTimeout(exec.Command("curl", "-fsSL", "-o", "/usr/local/bin/wp", "https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar"), 2*time.Minute); err != nil {
		return fmt.Errorf("download wp-cli: %s: %w", tail(out), err)
	}
	return os.Chmod("/usr/local/bin/wp", 0755)
}

func serviceActive(name string) bool {
	return exec.Command("systemctl", "is-active", "--quiet", name).Run() == nil
}

func phpAnyActive() bool {
	for _, v := range []string{"8.1", "8.2", "8.3", "8.4"} {
		if serviceActive("php" + v + "-fpm") {
			return true
		}
	}
	return false
}

func combinedTimeout(cmd *exec.Cmd, d time.Duration) (string, error) {
	ensureCmdHome(cmd)
	var buf strings.Builder
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Start(); err != nil {
		return "", err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return buf.String(), err
	case <-time.After(d):
		_ = cmd.Process.Kill()
		return buf.String(), fmt.Errorf("timed out after %s", d)
	}
}

func ensureCmdHome(cmd *exec.Cmd) {
	env := cmd.Env
	if env == nil {
		env = append([]string{}, os.Environ()...)
	}
	found := false
	for i, e := range env {
		if strings.HasPrefix(e, "HOME=") {
			if e == "HOME=" {
				env[i] = "HOME=/root"
			}
			found = true
			break
		}
	}
	if !found {
		env = append(env, "HOME=/root")
	}
	cmd.Env = env
}

func configureWAF(string) error {
	_ = exec.Command("a2enmod", "security2").Run()
	if _, err := os.Stat("/etc/modsecurity/modsecurity.conf"); err != nil {
		_ = copyFile("/etc/modsecurity/modsecurity.conf-recommended", "/etc/modsecurity/modsecurity.conf")
	}
	return security.EnsureWAF()
}

func configureClamAV(string) error {
	_ = os.MkdirAll("/var/log/clamav", 0755)
	_ = os.MkdirAll("/var/lib/clamav", 0755)
	_ = exec.Command("chown", "-R", "clamav:clamav", "/var/log/clamav", "/var/lib/clamav").Run()
	_ = exec.Command("systemctl", "enable", "--now", "clamav-freshclam").Run()
	_ = exec.Command("systemctl", "enable", "--now", "clamav-daemon").Run()
	return nil
}

func ufwActive() bool {
	out, err := exec.Command("ufw", "status").CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), "status: active")
}

func wafEnabled() bool {
	b, err := os.ReadFile("/etc/modsecurity/cp-engine.conf")
	if err != nil {
		b, err = os.ReadFile("/etc/modsecurity/modsecurity.conf")
		if err != nil {
			return false
		}
	}
	s := string(b)
	return strings.Contains(s, "SecRuleEngine On") || strings.Contains(s, "SecRuleEngine DetectionOnly")
}

func controlUFW(action string) error {
	switch action {
	case "start":
		security.ApplyDefaultUFW()
		out, err := exec.Command("ufw", "--force", "enable").CombinedOutput()
		if err != nil {
			return fmt.Errorf("ufw enable: %s: %w", strings.TrimSpace(string(out)), err)
		}
	case "stop":
		out, err := exec.Command("ufw", "disable").CombinedOutput()
		if err != nil {
			return fmt.Errorf("ufw disable: %s: %w", strings.TrimSpace(string(out)), err)
		}
	case "restart", "reload":
		_ = exec.Command("ufw", "reload").Run()
	default:
		return fmt.Errorf("invalid firewall action")
	}
	return nil
}

func controlWAF(action string) error {
	mode := "DetectionOnly"
	switch action {
	case "start":
		mode = "On"
	case "stop":
		mode = "Off"
	case "reload", "restart":
		mode = "DetectionOnly"
	default:
		return fmt.Errorf("invalid WAF action")
	}
	if err := os.WriteFile("/etc/modsecurity/cp-engine.conf", []byte("SecRuleEngine "+mode+"\n"), 0644); err != nil {
		return err
	}
	_ = exec.Command("systemctl", "reload", "apache2").Run()
	return nil
}

func copyFile(src, dest string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, b, 0644)
}

func tail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 800 {
		sc := bufio.NewScanner(strings.NewReader(s))
		var lines []string
		for sc.Scan() {
			lines = append(lines, sc.Text())
		}
		if len(lines) > 12 {
			lines = lines[len(lines)-12:]
		}
		return strings.Join(lines, "\n")
	}
	return s
}

package software

import (
	"regexp"
	"sort"
	"strings"
)

// AptManagedPath lists Debian packages Siroc installed. apt upgrade skips them.
const AptManagedPath = "/var/lib/siroc/apt-managed"

var (
	phpHoldRe    = regexp.MustCompile(`^php(\d+\.\d+)(?:-|$)`)
	pythonHoldRe = regexp.MustCompile(`^python(\d+\.\d+)(?:-|$)`)
)

// exactPanelPkg are catalog packages that do not share a versioned family.
var exactPanelPkg = map[string]struct{}{
	"ufw":                      {},
	"fail2ban":                 {},
	"git":                      {},
	"certbot":                  {},
	"rclone":                   {},
	"golang-go":                {},
	"rustc":                    {},
	"cargo":                    {},
	"docker.io":                {},
	"docker-compose-v2":        {},
	"openssh-server":           {},
	"vsftpd":                   {},
	"goaccess":                 {},
	"supervisor":               {},
	"nikto":                    {},
	"memcached":                {},
	"quota":                    {},
	"clamav":                   {},
	"clamav-daemon":            {},
	"clamav-freshclam":         {},
	"libapache2-mod-security2": {},
	"modsecurity-crs":          {},
}

func holdKey(pkg string) string {
	switch {
	case pkg == "nginx" || strings.HasPrefix(pkg, "nginx-") || strings.HasPrefix(pkg, "libnginx"):
		return "family:nginx"
	case pkg == "apache2" || strings.HasPrefix(pkg, "apache2-"):
		return "family:apache2"
	case pkg == "mysql-common":
		return "exact:mysql-common"
	case strings.HasPrefix(pkg, "mariadb-"):
		return "family:mariadb"
	case strings.HasPrefix(pkg, "mysql-"):
		return "family:mysql"
	case pkg == "redis" || strings.HasPrefix(pkg, "redis-"):
		return "family:redis"
	}
	if m := phpHoldRe.FindStringSubmatch(pkg); m != nil {
		return "family:php" + m[1]
	}
	if m := pythonHoldRe.FindStringSubmatch(pkg); m != nil {
		return "family:python" + m[1]
	}
	return "exact:" + pkg
}

func matchesHold(seed, pkg string) bool {
	sk := holdKey(seed)
	if sk == holdKey(pkg) {
		return true
	}
	if pkg == "mysql-common" && (sk == "family:mariadb" || sk == "family:mysql") {
		return true
	}
	return false
}

// panelManagedPkg reports packages the panel installs and then keeps off apt upgrade.
func panelManagedPkg(pkg string) bool {
	switch holdKey(pkg) {
	case "family:nginx", "family:apache2", "family:mariadb", "family:mysql", "family:redis":
		return true
	}
	key := holdKey(pkg)
	if strings.HasPrefix(key, "family:php") || strings.HasPrefix(key, "family:python") {
		return true
	}
	if pkg == "mysql-common" {
		return true
	}
	_, ok := exactPanelPkg[pkg]
	return ok
}

// ShouldSeed is false for OpenSSH and Python. Those match packages Ubuntu already
// ships, so they are held only after Software installs or updates them.
func ShouldSeed(pkg string) bool {
	if !panelManagedPkg(pkg) {
		return false
	}
	if pkg == "openssh-server" || strings.HasPrefix(pkg, "openssh-") {
		return false
	}
	if strings.HasPrefix(holdKey(pkg), "family:python") {
		return false
	}
	return true
}

// expandHold returns installed packages in the same family as seeds.
func expandHold(seeds, installed []string) []string {
	var out []string
	for _, pkg := range installed {
		for _, seed := range seeds {
			if matchesHold(seed, pkg) {
				out = append(out, pkg)
				break
			}
		}
	}
	return normalizePkgs(out)
}

func releaseNames(seeds, recorded, installed []string) []string {
	out := expandHold(seeds, installed)
	for _, name := range recorded {
		for _, seed := range seeds {
			if matchesHold(seed, name) {
				out = append(out, name)
				break
			}
		}
	}
	return normalizePkgs(out)
}

func parseManaged(text string) []string {
	var pkgs []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		pkgs = append(pkgs, line)
	}
	return normalizePkgs(pkgs)
}

func formatManaged(pkgs []string) string {
	pkgs = normalizePkgs(pkgs)
	var b strings.Builder
	b.WriteString("# Installed by Siroc. apt upgrade skips these. Update them from Software.\n")
	for _, p := range pkgs {
		b.WriteString(p)
		b.WriteByte('\n')
	}
	return b.String()
}

// debInstalledStatus is true for a package that is on disk. apt-mark hold
// changes the selection from "install" to "hold", so both count.
func debInstalledStatus(status string) bool {
	return strings.Contains(status, " ok installed")
}

func normalizePkgs(pkgs []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, p := range pkgs {
		p = strings.TrimSpace(p)
		if p == "" || strings.HasPrefix(p, "#") {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	sort.Strings(out)
	if out == nil {
		return []string{}
	}
	return out
}

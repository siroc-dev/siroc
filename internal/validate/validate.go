package validate

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var (
	userRe   = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,31}$`)
	domainRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`)
	dbRe     = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{2,63}$`)
)

var reservedUsers = map[string]struct{}{
	"root": {}, "daemon": {}, "bin": {}, "sys": {}, "sync": {}, "games": {},
	"man": {}, "lp": {}, "mail": {}, "news": {}, "uucp": {}, "proxy": {},
	"www-data": {}, "backup": {}, "list": {}, "nobody": {}, "systemd-network": {},
	"systemd-resolve": {}, "messagebus": {}, "sshd": {}, "mysql": {}, "mariadb": {},
	"redis": {}, "nginx": {}, "apache": {}, "siroc": {}, "cpserver": {}, "admin": {},
	"ftp": {}, "vsftpd": {}, "ubuntu": {},
}

var ftpNameRe = regexp.MustCompile(`^[a-z][a-z0-9]{1,15}$`)

func FTPName(name string) error {
	if !ftpNameRe.MatchString(name) {
		return fmt.Errorf("ftp name must be 2-16 chars, start with a letter, and use a-z or digits")
	}
	return nil
}

func FTPLogin(owner, name string) (string, error) {
	if err := LinuxUser(owner); err != nil {
		return "", err
	}
	if err := FTPName(name); err != nil {
		return "", err
	}
	login := owner + "_" + name
	if err := LinuxUser(login); err != nil {
		return "", fmt.Errorf("ftp login %q is invalid (too long?)", login)
	}
	return login, nil
}

func LinuxUser(name string) error {
	if !userRe.MatchString(name) {
		return fmt.Errorf("username must be 3-32 chars, start with a letter, and use a-z, 0-9, _ or -")
	}
	if _, ok := reservedUsers[name]; ok {
		return fmt.Errorf("username %q is reserved", name)
	}
	return nil
}

func Domain(name string) error {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "localhost" {
		return nil
	}
	if !domainRe.MatchString(n) {
		return fmt.Errorf("invalid domain")
	}
	if strings.Contains(n, "..") {
		return fmt.Errorf("invalid domain")
	}
	return nil
}

func WildcardAlias(name string) error {
	n := strings.ToLower(strings.TrimSpace(name))
	if !strings.HasPrefix(n, "*.") {
		return fmt.Errorf("invalid domain")
	}
	rest := strings.TrimPrefix(n, "*.")
	if rest == "" || strings.Contains(rest, "*") {
		return fmt.Errorf("invalid domain")
	}
	return Domain(rest)
}

func DomainOrWildcardAlias(name string) error {
	n := strings.ToLower(strings.TrimSpace(name))
	if strings.HasPrefix(n, "*.") {
		return WildcardAlias(n)
	}
	return Domain(n)
}

func DomainAliases(primary string, aliases []string) ([]string, error) {
	primary = strings.ToLower(strings.TrimSpace(primary))
	if err := Domain(primary); err != nil {
		return nil, err
	}
	seen := map[string]struct{}{primary: {}}
	var out []string
	for _, raw := range aliases {
		n := strings.ToLower(strings.TrimSpace(raw))
		if n == "" {
			continue
		}
		if err := DomainOrWildcardAlias(n); err != nil {
			return nil, fmt.Errorf("alias %q: %w", n, err)
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	if len(out) > 20 {
		return nil, fmt.Errorf("at most 20 domain aliases")
	}
	if out == nil {
		out = []string{}
	}
	return out, nil
}

var laravelQueueNameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

func LaravelQueueNames(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "default", nil
	}
	seen := map[string]bool{}
	var names []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !laravelQueueNameRe.MatchString(part) {
			return "", fmt.Errorf("invalid queue name %q", part)
		}
		if seen[part] {
			continue
		}
		seen[part] = true
		names = append(names, part)
	}
	if len(names) == 0 {
		return "default", nil
	}
	if len(names) > 8 {
		return "", fmt.Errorf("at most 8 queue names")
	}
	return strings.Join(names, ","), nil
}

func LaravelQueueWorkers(n int) (int, error) {
	if n < 1 {
		return 1, nil
	}
	if n > 8 {
		return 0, fmt.Errorf("at most 8 workers per queue")
	}
	return n, nil
}

func Email(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	if len(v) > 120 || !strings.Contains(v, "@") || strings.Contains(v, " ") {
		return fmt.Errorf("invalid email")
	}
	return nil
}

func ScanTarget(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("target required")
	}
	if strings.ContainsAny(raw, " \t\n\r") {
		return "", fmt.Errorf("invalid target")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid target URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("target must be an http(s) URL")
	}
	if u.User != nil {
		return "", fmt.Errorf("target must not contain credentials")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", fmt.Errorf("invalid target host")
	}
	if host != "localhost" && net.ParseIP(host) == nil {
		if err := Domain(host); err != nil {
			return "", fmt.Errorf("invalid target host")
		}
	}
	return u.String(), nil
}

func RelUploadPath(rel string) (string, error) {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		return "", nil
	}
	if strings.ContainsRune(rel, 0) || strings.Contains(rel, "\\") {
		return "", fmt.Errorf("invalid relative path")
	}
	var parts []string
	for _, p := range strings.Split(rel, "/") {
		if p == "" || p == "." {
			continue
		}
		if p == ".." {
			return "", fmt.Errorf("invalid relative path")
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, "/"), nil
}

func NginxSnippet(raw string) (string, error) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	if strings.ContainsRune(raw, 0) {
		return "", fmt.Errorf("invalid nginx rewrite")
	}
	if len(raw) > 16384 {
		return "", fmt.Errorf("nginx rewrite is too long")
	}
	var out []string
	depth := 0
	n := 0
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		n++
		if n > 200 {
			return "", fmt.Errorf("at most 200 nginx rewrite lines")
		}
		if strings.HasPrefix(line, "#") {
			out = append(out, line)
			continue
		}
		low := strings.ToLower(line)
		for _, bad := range []string{
			"include ", "proxy_pass", "fastcgi_", "uwsgi_", "scgi_", "grpc_",
			"memcached_", "mirror ", "dav_", "perl", "lua", "js_", "load_module",
			"ssl_certificate", "listen ", "server_name", "root ", "alias ",
			"access_log", "error_log", "client_body", "stub_status", "auth_basic",
			"location ", "http ", "server ", "upstream ", "map ",
		} {
			if strings.Contains(low, bad) {
				return "", fmt.Errorf("nginx rewrite cannot use %s", strings.TrimSpace(bad))
			}
		}
		open := strings.Count(line, "{")
		close := strings.Count(line, "}")
		depth += open - close
		if depth < 0 {
			return "", fmt.Errorf("unbalanced braces in nginx rewrite")
		}
		out = append(out, line)
	}
	if depth != 0 {
		return "", fmt.Errorf("unbalanced braces in nginx rewrite")
	}
	if len(out) == 0 {
		return "", nil
	}
	var b strings.Builder
	for _, line := range out {
		b.WriteString("        ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String(), nil
}

func DBIdent(name string) error {
	if !dbRe.MatchString(name) {
		return fmt.Errorf("database name must start with a letter and use A-Z, 0-9, _")
	}
	return nil
}

func PHPVersion(v string) error {
	switch v {
	case "8.1", "8.2", "8.3", "8.4":
		return nil
	default:
		return fmt.Errorf("unsupported PHP version")
	}
}

var (
	phpExtRe  = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,40}$`)
	phpSizeRe = regexp.MustCompile(`(?i)^\d+[KMG]?$`)
	phpTZRe   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_+\-/]{0,63}$`)
	phpIdleRe = regexp.MustCompile(`^\d+[sm]?$`)
	phpFnRe   = regexp.MustCompile(`^[a-zA-Z0-9_, ]*$`)
)

func PHPExtName(name string) error {
	n := strings.ToLower(strings.TrimSpace(name))
	if !phpExtRe.MatchString(n) {
		return fmt.Errorf("invalid PHP extension name")
	}
	if strings.Contains(n, "..") {
		return fmt.Errorf("invalid PHP extension name")
	}
	return nil
}

func PHPSize(v, field string) error {
	if !phpSizeRe.MatchString(strings.TrimSpace(v)) {
		return fmt.Errorf("invalid %s (use e.g. 256M)", field)
	}
	return nil
}

func PHPTimezone(v string) error {
	if !phpTZRe.MatchString(strings.TrimSpace(v)) {
		return fmt.Errorf("invalid timezone")
	}
	return nil
}

func PHPIdleTimeout(v string) error {
	if !phpIdleRe.MatchString(strings.TrimSpace(v)) {
		return fmt.Errorf("invalid idle timeout (use e.g. 10s)")
	}
	return nil
}

func PHPDisableFunctions(v string) error {
	if !phpFnRe.MatchString(v) {
		return fmt.Errorf("invalid disable_functions list")
	}
	return nil
}

func PHPFPM(pm string, maxChildren, startServers, minSpare, maxSpare, maxRequests, maxExec, maxInput int) error {
	switch pm {
	case "ondemand", "dynamic", "static":
	default:
		return fmt.Errorf("pm must be ondemand, dynamic, or static")
	}
	if maxChildren < 1 || maxChildren > 256 {
		return fmt.Errorf("max children must be 1-256")
	}
	if startServers < 1 || startServers > maxChildren {
		return fmt.Errorf("start servers must be 1-%d", maxChildren)
	}
	if minSpare < 1 || minSpare > maxChildren {
		return fmt.Errorf("min spare servers must be 1-%d", maxChildren)
	}
	if maxSpare < minSpare || maxSpare > maxChildren {
		return fmt.Errorf("max spare servers must be %d-%d", minSpare, maxChildren)
	}
	if maxRequests < 0 || maxRequests > 100000 {
		return fmt.Errorf("max requests must be 0-100000")
	}
	if maxExec < 0 || maxExec > 3600 {
		return fmt.Errorf("max execution time must be 0-3600")
	}
	if maxInput < 0 || maxInput > 3600 {
		return fmt.Errorf("max input time must be 0-3600")
	}
	return nil
}

func TermCWD(homeRoot, username, raw string, asRoot bool) (string, error) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if raw == "" {
		return "", nil
	}
	if strings.ContainsRune(raw, 0) {
		return "", fmt.Errorf("invalid cwd")
	}
	if asRoot {
		if !strings.HasPrefix(raw, "/") {
			return "", fmt.Errorf("cwd must be absolute")
		}
		p := filepath.Clean(raw)
		if !filepath.IsAbs(p) {
			return "", fmt.Errorf("invalid cwd")
		}
		return p, nil
	}
	return AccountPath(homeRoot, username, raw, "")
}

func AccountPath(homeRoot, username, raw, domain string) (string, error) {
	if err := LinuxUser(username); err != nil {
		return "", err
	}
	home := filepath.Clean(filepath.Join(homeRoot, username))
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if raw == "" {
		if domain == "" {
			return "", fmt.Errorf("document root is required")
		}
		raw = filepath.Join("domains", domain, "public_html")
	}
	var abs string
	if filepath.IsAbs(raw) {
		abs = filepath.Clean(raw)
	} else {
		abs = filepath.Clean(filepath.Join(home, raw))
	}
	rel, err := filepath.Rel(home, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path must stay inside the account home")
	}
	return abs, nil
}

func RelHome(homeRoot, username, abs string) string {
	home := filepath.Clean(filepath.Join(homeRoot, username))
	rel, err := filepath.Rel(home, filepath.Clean(abs))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return abs
	}
	return filepath.ToSlash(rel)
}

func HomeJail(homeRoot, username string) (home, tmp string, err error) {
	if err := LinuxUser(username); err != nil {
		return "", "", err
	}
	home = filepath.Clean(filepath.Join(homeRoot, username))
	return home, filepath.Join(home, "tmp"), nil
}

func Printable(s string) bool {
	for _, r := range s {
		if r == 0 || (!unicode.IsPrint(r) && r != '\n' && r != '\r' && r != '\t') {
			return false
		}
	}
	return true
}

func ProxyURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", fmt.Errorf("proxy URL required")
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("proxy URL must be http:// or https://host[:port][/path]")
	}
	if strings.ContainsAny(s, " \n\t;{}") {
		return "", fmt.Errorf("invalid proxy URL")
	}
	if !strings.HasSuffix(s, "/") && u.Path == "" {
		s += "/"
	}
	return s, nil
}

func SiteKind(kind string) (string, error) {
	k := strings.ToLower(strings.TrimSpace(kind))
	switch k {
	case "", "php":
		return "php", nil
	case "proxy", "nodejs", "python", "go", "rust", "docker":
		return k, nil
	default:
		return "", fmt.Errorf("unknown site type %q", kind)
	}
}

func IsAppKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "nodejs", "python", "go", "rust", "docker":
		return true
	default:
		return false
	}
}

func DefaultAppCmd(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "nodejs":
		return "node server.js"
	case "python":
		return "python3 app.py"
	case "go":
		return "go run ."
	case "rust":
		return "cargo run --release"
	case "docker":
		return "docker compose up --build --abort-on-container-exit"
	default:
		return ""
	}
}

func AppCommand(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("start command required")
	}
	if len(s) > 500 {
		return "", fmt.Errorf("start command too long")
	}
	if strings.ContainsAny(s, "\n\r\x00") {
		return "", fmt.Errorf("invalid start command")
	}
	return s, nil
}

func AppPort(n int) error {
	if n < 1024 || n > 65535 {
		return fmt.Errorf("app port must be 1024-65535")
	}
	switch n {
	case 80, 443, 8080, 8443, 3306, 5432, 6379:
		return fmt.Errorf("port %d is reserved", n)
	}
	return nil
}

package siteopts

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/siroc-dev/siroc/internal/validate"
)

// Options are per-site nginx/apache settings edited from the site dialog.
type Options struct {
	Index   []string `json:"index,omitempty"`
	Access  string   `json:"access,omitempty"`
	IPs     []string `json:"ips,omitempty"`
	Hotlink bool     `json:"hotlink,omitempty"`
	// HotlinkDomains are extra referer hosts. The site name and its aliases are always allowed.
	HotlinkDomains []string   `json:"hotlinkDomains,omitempty"`
	Maintenance    bool       `json:"maintenance,omitempty"`
	Redirects      []Redirect `json:"redirects,omitempty"`
	Proxy          *Proxy     `json:"proxy,omitempty"`
	// Static is nil when the site still uses the default (Nginx serves static files).
	// A false pointer turns that off. StaticExt empty means the Plesk extension list.
	Static    *bool  `json:"static,omitempty"`
	StaticExt string `json:"staticExt,omitempty"`
	// ForceHTTPS nil keeps the previous default: redirect when the certificate is Let's Encrypt or custom.
	ForceHTTPS *bool `json:"forceHttps,omitempty"`
	// PHP overrides this site's pool. Empty fields keep the account PHP settings.
	PHP *PHP `json:"php,omitempty"`
	// ComposerRoot is the directory that holds composer.json. Empty uses the app directory above the document root when that file is there.
	ComposerRoot string `json:"composerRoot,omitempty"`
	// AccessLog and ErrorLog are nil when that log stays on. A false pointer turns that log off.
	AccessLog *bool `json:"accessLog,omitempty"`
	ErrorLog  *bool `json:"errorLog,omitempty"`
	// Cache stores responses nginx fetches from Apache or a reverse proxy.
	Cache *Cache `json:"cache,omitempty"`
}

// PHP is a per-site php.ini override applied to this site's FPM pool.
type PHP struct {
	MemoryLimit       string `json:"memoryLimit,omitempty"`
	MaxExecutionTime  int    `json:"maxExecutionTime,omitempty"`
	MaxInputTime      int    `json:"maxInputTime,omitempty"`
	PostMaxSize       string `json:"postMaxSize,omitempty"`
	UploadMaxFilesize string `json:"uploadMaxFilesize,omitempty"`
	Timezone          string `json:"timezone,omitempty"`
	DisplayErrors     *bool  `json:"displayErrors,omitempty"`
	DisableFunctions  string `json:"disableFunctions,omitempty"`
}

type Redirect struct {
	From string `json:"from"`
	To   string `json:"to"`
	Code int    `json:"code"`
}

func Parse(raw string) Options {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" || raw == "null" {
		return Options{}
	}
	var o Options
	if err := json.Unmarshal([]byte(raw), &o); err != nil {
		return Options{}
	}
	norm, err := Normalize(o)
	if err != nil {
		return Options{}
	}
	return norm
}

func Marshal(o Options) (string, error) {
	norm, err := Normalize(o)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(norm)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func IsZero(o Options) bool {
	return len(o.Index) == 0 && o.Access == "" && len(o.IPs) == 0 && !o.Hotlink && len(o.HotlinkDomains) == 0 && !o.Maintenance && len(o.Redirects) == 0 && o.Proxy == nil && !staticExplicitOff(o) && o.StaticExt == "" && o.ForceHTTPS == nil && o.PHP == nil && o.ComposerRoot == "" && o.AccessLog == nil && o.ErrorLog == nil && o.Cache == nil
}

func Normalize(in Options) (Options, error) {
	var out Options
	seenIdx := map[string]struct{}{}
	for _, raw := range in.Index {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if !indexName(name) {
			return Options{}, fmt.Errorf("default document %q is not a file name", name)
		}
		if _, ok := seenIdx[name]; ok {
			continue
		}
		seenIdx[name] = struct{}{}
		out.Index = append(out.Index, name)
		if len(out.Index) > 8 {
			return Options{}, fmt.Errorf("at most 8 default documents")
		}
	}
	access := strings.ToLower(strings.TrimSpace(in.Access))
	switch access {
	case "", "allow", "deny":
	default:
		return Options{}, fmt.Errorf("access must be allow or deny")
	}
	seenIP := map[string]struct{}{}
	for _, raw := range in.IPs {
		ip := strings.TrimSpace(raw)
		if ip == "" {
			continue
		}
		canon, err := canonIP(ip)
		if err != nil {
			return Options{}, err
		}
		if _, ok := seenIP[canon]; ok {
			continue
		}
		seenIP[canon] = struct{}{}
		out.IPs = append(out.IPs, canon)
		if len(out.IPs) > 40 {
			return Options{}, fmt.Errorf("at most 40 access addresses")
		}
	}
	if access == "allow" && len(out.IPs) == 0 {
		return Options{}, fmt.Errorf("allow list needs at least one address")
	}
	if access == "deny" && len(out.IPs) == 0 {
		return Options{}, fmt.Errorf("deny list needs at least one address")
	}
	out.Access = access
	out.Hotlink = in.Hotlink
	hosts, err := cleanHotlinkDomains(in.HotlinkDomains)
	if err != nil {
		return Options{}, err
	}
	out.HotlinkDomains = hosts
	out.Maintenance = in.Maintenance
	seenFrom := map[string]struct{}{}
	for _, r := range in.Redirects {
		from := strings.TrimSpace(r.From)
		to := strings.TrimSpace(r.To)
		if from == "" && to == "" {
			continue
		}
		if !validFrom(from) {
			return Options{}, fmt.Errorf("redirect path %q is invalid", from)
		}
		if !validTarget(to) {
			return Options{}, fmt.Errorf("redirect target %q is invalid", to)
		}
		code := r.Code
		if code == 0 {
			code = 301
		}
		if code != 301 && code != 302 {
			return Options{}, fmt.Errorf("redirect code must be 301 or 302")
		}
		if _, ok := seenFrom[from]; ok {
			return Options{}, fmt.Errorf("duplicate redirect %s", from)
		}
		seenFrom[from] = struct{}{}
		out.Redirects = append(out.Redirects, Redirect{From: from, To: to, Code: code})
		if len(out.Redirects) > 20 {
			return Options{}, fmt.Errorf("at most 20 redirects")
		}
	}
	p, err := normalizeProxy(in.Proxy)
	if err != nil {
		return Options{}, err
	}
	out.Proxy = p
	if in.Static != nil {
		v := *in.Static
		out.Static = &v
	}
	ext, err := parseStaticExt(in.StaticExt)
	if err != nil {
		return Options{}, err
	}
	out.StaticExt = strings.Join(ext, " ")
	if in.ForceHTTPS != nil {
		v := *in.ForceHTTPS
		out.ForceHTTPS = &v
	}
	php, err := normalizePHP(in.PHP)
	if err != nil {
		return Options{}, err
	}
	out.PHP = php
	root, err := cleanComposerRoot(in.ComposerRoot)
	if err != nil {
		return Options{}, err
	}
	out.ComposerRoot = root
	out.AccessLog = offOnly(in.AccessLog)
	out.ErrorLog = offOnly(in.ErrorLog)
	cache, err := normalizeSiteCache(in.Cache)
	if err != nil {
		return Options{}, err
	}
	out.Cache = cache
	return out, nil
}

func offOnly(p *bool) *bool {
	if p == nil || *p {
		return nil
	}
	v := false
	return &v
}

func LogOn(p *bool) bool {
	return p == nil || *p
}

func normalizePHP(in *PHP) (*PHP, error) {
	if in == nil {
		return nil, nil
	}
	out := &PHP{}
	if s := strings.TrimSpace(in.MemoryLimit); s != "" {
		if err := validate.PHPSize(s, "memory_limit"); err != nil {
			return nil, err
		}
		out.MemoryLimit = s
	}
	if in.MaxExecutionTime != 0 {
		if in.MaxExecutionTime < 0 || in.MaxExecutionTime > 3600 {
			return nil, fmt.Errorf("max execution time must be 1-3600")
		}
		out.MaxExecutionTime = in.MaxExecutionTime
	}
	if in.MaxInputTime != 0 {
		if in.MaxInputTime < 0 || in.MaxInputTime > 3600 {
			return nil, fmt.Errorf("max input time must be 1-3600")
		}
		out.MaxInputTime = in.MaxInputTime
	}
	if s := strings.TrimSpace(in.PostMaxSize); s != "" {
		if err := validate.PHPSize(s, "post_max_size"); err != nil {
			return nil, err
		}
		out.PostMaxSize = s
	}
	if s := strings.TrimSpace(in.UploadMaxFilesize); s != "" {
		if err := validate.PHPSize(s, "upload_max_filesize"); err != nil {
			return nil, err
		}
		out.UploadMaxFilesize = s
	}
	if s := strings.TrimSpace(in.Timezone); s != "" {
		if err := validate.PHPTimezone(s); err != nil {
			return nil, err
		}
		out.Timezone = s
	}
	if in.DisplayErrors != nil {
		v := *in.DisplayErrors
		out.DisplayErrors = &v
	}
	if s := strings.TrimSpace(in.DisableFunctions); s != "" {
		if err := validate.PHPDisableFunctions(s); err != nil {
			return nil, err
		}
		out.DisableFunctions = s
	}
	if out.MemoryLimit == "" && out.MaxExecutionTime == 0 && out.MaxInputTime == 0 && out.PostMaxSize == "" && out.UploadMaxFilesize == "" && out.Timezone == "" && out.DisplayErrors == nil && out.DisableFunctions == "" {
		return nil, nil
	}
	return out, nil
}

func cleanComposerRoot(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if raw == "" {
		return "", nil
	}
	if strings.ContainsAny(raw, "\r\n\x00") || strings.Contains(raw, "..") {
		return "", fmt.Errorf("invalid composer path")
	}
	if len(raw) > 200 {
		return "", fmt.Errorf("composer path is too long")
	}
	return raw, nil
}

func cleanHotlinkDomains(in []string) ([]string, error) {
	seen := map[string]struct{}{}
	var out []string
	for _, raw := range in {
		name, err := hotlinkName(raw)
		if err != nil {
			return nil, err
		}
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
		if len(out) > 40 {
			return nil, fmt.Errorf("at most 40 hotlink domains")
		}
	}
	return out, nil
}

func hotlinkName(raw string) (string, error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	raw = strings.TrimPrefix(raw, "https://")
	raw = strings.TrimPrefix(raw, "http://")
	if i := strings.IndexAny(raw, "/?#"); i >= 0 {
		raw = raw[:i]
	}
	raw = strings.TrimSuffix(raw, ".")
	if host, _, err := net.SplitHostPort(raw); err == nil {
		raw = host
	}
	if raw == "" {
		return "", nil
	}
	wild := strings.HasPrefix(raw, "*.")
	check := raw
	if wild {
		check = strings.TrimPrefix(raw, "*.")
	}
	ascii, err := validate.ASCIIHost(check)
	if err != nil {
		return "", fmt.Errorf("hotlink domain %q is invalid", strings.TrimSpace(raw))
	}
	if wild {
		return "*." + ascii, nil
	}
	return ascii, nil
}

// NginxExtra is a server-context snippet: maintenance, redirects, and hotlink.
// proxy is the site's proxy_pass; empty means Apache on 127.0.0.1:8080.
func NginxExtra(o Options, proxy string) (string, error) {
	norm, err := Normalize(o)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if norm.Maintenance {
		b.WriteString("    if ($request_uri !~ \"^/.well-known/acme-challenge/\") {\n")
		b.WriteString("        return 503;\n")
		b.WriteString("    }\n")
	}
	for _, r := range norm.Redirects {
		fmt.Fprintf(&b, "    location = %s {\n        return %d %s;\n    }\n", r.From, r.Code, r.To)
	}
	if norm.Hotlink {
		backend, err := hotlinkBackend(proxy)
		if err != nil {
			return "", err
		}
		b.WriteString("    location ~* \\.(gif|jpg|jpeg|png|webp|svg|ico|css|js)$ {\n")
		b.WriteString("        valid_referers none blocked server_names")
		for _, name := range norm.HotlinkDomains {
			b.WriteByte(' ')
			b.WriteString(name)
		}
		b.WriteString(";\n")
		b.WriteString("        if ($invalid_referer) {\n")
		b.WriteString("            return 403;\n")
		b.WriteString("        }\n")
		if strings.TrimSpace(proxy) == "" && StaticOn(norm) {
			b.WriteString("        try_files $uri @siroc_apache;\n")
		} else {
			fmt.Fprintf(&b, "        proxy_pass %s;\n", backend)
			b.WriteString("        proxy_set_header Host $http_host;\n")
			b.WriteString("        proxy_set_header X-Real-IP $remote_addr;\n")
			b.WriteString("        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n")
			b.WriteString("        proxy_set_header X-Forwarded-Proto $scheme;\n")
		}
		b.WriteString("    }\n")
	}
	return b.String(), nil
}

// NginxAccess is server-context allow/deny so every location inherits it. Empty when off.
func NginxAccess(o Options) (string, error) {
	norm, err := Normalize(o)
	if err != nil {
		return "", err
	}
	if norm.Access == "" {
		return "", nil
	}
	var b strings.Builder
	if norm.Access == "deny" {
		for _, ip := range norm.IPs {
			fmt.Fprintf(&b, "    deny %s;\n", ip)
		}
		return b.String(), nil
	}
	for _, ip := range norm.IPs {
		fmt.Fprintf(&b, "    allow %s;\n", ip)
	}
	b.WriteString("    deny all;\n")
	return b.String(), nil
}

// ApacheIndex is a DirectoryIndex argument list, or empty for the server default.
func ApacheIndex(o Options) (string, error) {
	norm, err := Normalize(o)
	if err != nil {
		return "", err
	}
	return strings.Join(norm.Index, " "), nil
}

func indexName(name string) bool {
	if len(name) == 0 || len(name) > 40 || strings.Contains(name, "..") {
		return false
	}
	for i, r := range name {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-'
		if i == 0 && (r == '.' || r == '-' || r == '_') {
			return false
		}
		if !ok {
			return false
		}
	}
	return true
}

func canonIP(raw string) (string, error) {
	if strings.Contains(raw, "/") {
		ip, n, err := net.ParseCIDR(raw)
		if err != nil || ip == nil {
			return "", fmt.Errorf("invalid address %q", raw)
		}
		return n.String(), nil
	}
	ip := net.ParseIP(raw)
	if ip == nil {
		return "", fmt.Errorf("invalid address %q", raw)
	}
	return ip.String(), nil
}

func validFrom(p string) bool {
	if !validPath(p) {
		return false
	}
	if strings.HasPrefix(p, "/.well-known/") || p == "/.well-known" {
		return false
	}
	return true
}

func validPath(p string) bool {
	if len(p) < 1 || len(p) > 200 || p[0] != '/' || strings.Contains(p, "..") {
		return false
	}
	return !strings.ContainsAny(p, " \t\r\n;{}'\"`$\\?")
}

func validTarget(p string) bool {
	if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
		if len(p) > 500 || strings.ContainsAny(p, " \t\r\n;{}'\"`$\\") {
			return false
		}
		u, err := url.Parse(p)
		return err == nil && u.Host != "" && u.Hostname() != ""
	}
	return validPath(p)
}

func hotlinkBackend(proxy string) (string, error) {
	proxy = strings.TrimSpace(proxy)
	if proxy == "" {
		return "http://127.0.0.1:8080", nil
	}
	if strings.ContainsAny(proxy, " \t\r\n;{}'\"`$\\") {
		return "", fmt.Errorf("invalid proxy target")
	}
	u, err := url.Parse(proxy)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("invalid proxy target")
	}
	return proxy, nil
}

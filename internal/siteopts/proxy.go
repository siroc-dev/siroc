package siteopts

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"path"
	"strings"
)

const (
	DefaultProxyCachePath = "/var/cache/nginx/siroc"
	DefaultProxyCacheZone = "siroc_cache"
)

var deniedCachePaths = []string{
	"/bin", "/boot", "/dev", "/etc", "/lib", "/lib32", "/lib64", "/libx32",
	"/proc", "/root", "/run", "/sbin", "/snap", "/sys", "/tmp", "/usr", "/home",
	"/var/backups", "/var/lib", "/var/lock", "/var/log", "/var/run", "/var/spool",
}

// Proxy is the aaPanel-style reverse proxy for one site.
type Proxy struct {
	ShowPath       bool           `json:"showPath,omitempty"`
	Path           string         `json:"path,omitempty"`
	Target         string         `json:"target,omitempty"`
	Host           string         `json:"host,omitempty"`
	Rewrites       []ProxyRewrite `json:"rewrites,omitempty"`
	Remark         string         `json:"remark,omitempty"`
	Websocket      bool           `json:"websocket,omitempty"`
	ConnectTimeout int            `json:"connectTimeout,omitempty"`
	SendTimeout    int            `json:"sendTimeout,omitempty"`
	ReadTimeout    int            `json:"readTimeout,omitempty"`
	Config         string         `json:"config,omitempty"`
	Replacements   []ProxyReplace `json:"replacements,omitempty"`
	Cache          bool           `json:"cache,omitempty"`
	CachePath      string         `json:"cachePath,omitempty"`
	Gzip           bool           `json:"gzip,omitempty"`
	Black          []string       `json:"black,omitempty"`
	White          []string       `json:"white,omitempty"`
}

type ProxyRewrite struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type ProxyReplace struct {
	From string `json:"from"`
	To   string `json:"to"`
	Rule string `json:"rule,omitempty"`
}

func normalizeProxy(in *Proxy) (*Proxy, error) {
	if in == nil {
		return nil, nil
	}
	out := &Proxy{
		ShowPath:       in.ShowPath,
		Websocket:      in.Websocket,
		Cache:          in.Cache,
		Gzip:           in.Gzip,
		ConnectTimeout: clampTimeout(in.ConnectTimeout, 60, 3600),
		SendTimeout:    clampTimeout(in.SendTimeout, 600, 86400),
		ReadTimeout:    clampTimeout(in.ReadTimeout, 600, 86400),
	}
	out.Path = strings.TrimSpace(in.Path)
	if out.Path == "" {
		out.Path = "/"
	}
	if !validPath(out.Path) && out.Path != "/" {
		return nil, fmt.Errorf("proxy path %q is invalid", in.Path)
	}
	target := strings.TrimSpace(in.Target)
	if target != "" {
		if _, err := parseProxyTarget(target); err != nil {
			return nil, err
		}
		out.Target = target
	}
	host := strings.TrimSpace(in.Host)
	if host != "" && host != "$host" && host != "$http_host" {
		if strings.ContainsAny(host, " \t\r\n;{}'\"`$\\") || len(host) > 253 {
			return nil, fmt.Errorf("send host %q is invalid", in.Host)
		}
	}
	out.Host = host
	remark := strings.TrimSpace(in.Remark)
	if len(remark) > 200 || strings.ContainsAny(remark, "\r\n") {
		return nil, fmt.Errorf("remark is too long")
	}
	out.Remark = remark
	for _, rw := range in.Rewrites {
		from := strings.TrimSpace(rw.From)
		to := strings.TrimSpace(rw.To)
		if from == "" && to == "" {
			continue
		}
		if !validPath(from) || !validPath(to) {
			return nil, fmt.Errorf("URL rewrite %q → %q is invalid", rw.From, rw.To)
		}
		out.Rewrites = append(out.Rewrites, ProxyRewrite{From: from, To: to})
		if len(out.Rewrites) > 20 {
			return nil, fmt.Errorf("at most 20 URL rewrites")
		}
	}
	cfg, err := proxyConfig(in.Config)
	if err != nil {
		return nil, err
	}
	out.Config = cfg
	for _, rp := range in.Replacements {
		from := strings.TrimSpace(rp.From)
		to := strings.TrimSpace(rp.To)
		if from == "" && to == "" {
			continue
		}
		if !proxyText(from) || !proxyText(to) {
			return nil, fmt.Errorf("replacement %q is invalid", rp.From)
		}
		rule := strings.ToLower(strings.TrimSpace(rp.Rule))
		if rule == "" {
			rule = "g"
		}
		for _, r := range rule {
			if r != 'g' && r != 'i' && r != 'o' && r != 'r' {
				return nil, fmt.Errorf("replacement rule %q is invalid", rp.Rule)
			}
		}
		out.Replacements = append(out.Replacements, ProxyReplace{From: from, To: to, Rule: rule})
		if len(out.Replacements) > 20 {
			return nil, fmt.Errorf("at most 20 content replacements")
		}
	}
	out.CachePath, err = normalizeCachePath(in.CachePath)
	if err != nil {
		return nil, err
	}
	out.Black, err = proxyIPs(in.Black)
	if err != nil {
		return nil, err
	}
	out.White, err = proxyIPs(in.White)
	if err != nil {
		return nil, err
	}
	if proxyEmpty(out) {
		return nil, nil
	}
	return out, nil
}

func proxyEmpty(p *Proxy) bool {
	return p.Target == "" && p.Host == "" && len(p.Rewrites) == 0 && p.Remark == "" && p.Config == "" &&
		len(p.Replacements) == 0 && !p.Cache && p.CachePath == "" && !p.Gzip && len(p.Black) == 0 && len(p.White) == 0 &&
		p.Path == "/" && !p.ShowPath && !p.Websocket && p.ConnectTimeout == 60 && p.SendTimeout == 600 && p.ReadTimeout == 600
}

func clampTimeout(v, def, max int) int {
	if v <= 0 {
		return def
	}
	if v > max {
		return max
	}
	return v
}

func proxyIPs(in []string) ([]string, error) {
	var out []string
	seen := map[string]struct{}{}
	for _, raw := range in {
		ip := strings.TrimSpace(raw)
		if ip == "" {
			continue
		}
		canon, err := canonIP(ip)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[canon]; ok {
			continue
		}
		seen[canon] = struct{}{}
		out = append(out, canon)
		if len(out) > 40 {
			return nil, fmt.Errorf("at most 40 proxy addresses")
		}
	}
	return out, nil
}

func proxyText(s string) bool {
	if s == "" || len(s) > 200 || strings.ContainsAny(s, "\r\n'\"\\$`;{}") {
		return false
	}
	return true
}

func proxyConfig(raw string) (string, error) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	var lines []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if len(line) > 500 || strings.ContainsAny(line, "{}`") || !strings.HasSuffix(line, ";") {
			return "", fmt.Errorf("proxy config line %q is not allowed", line)
		}
		lower := strings.ToLower(line)
		ok := strings.HasPrefix(lower, "proxy_") || strings.HasPrefix(lower, "add_header ") ||
			strings.HasPrefix(lower, "gzip") || strings.HasPrefix(lower, "client_") || strings.HasPrefix(lower, "limit_rate ")
		if !ok || strings.Contains(lower, "include ") || strings.Contains(lower, "alias ") || strings.Contains(lower, "root ") {
			return "", fmt.Errorf("proxy config line %q is not allowed", line)
		}
		lines = append(lines, line)
		if len(lines) > 40 {
			return "", fmt.Errorf("at most 40 proxy config lines")
		}
	}
	return strings.Join(lines, "\n"), nil
}

func normalizeCachePath(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if raw == "" || raw == DefaultProxyCachePath {
		return "", nil
	}
	if !strings.HasPrefix(raw, "/") || strings.ContainsAny(raw, " \t\r\n;{}'\"`$") {
		return "", fmt.Errorf("cache path must be an absolute directory")
	}
	abs := path.Clean(raw)
	if !strings.HasPrefix(abs, "/") || abs == "/" {
		return "", fmt.Errorf("cache path must be a directory")
	}
	if strings.Count(strings.Trim(abs, "/"), "/") < 1 {
		return "", fmt.Errorf("cache path must be a directory on the disk, not the disk itself")
	}
	for _, prefix := range deniedCachePaths {
		if abs == prefix || strings.HasPrefix(abs, prefix+"/") {
			return "", fmt.Errorf("cache path cannot be under %s", prefix)
		}
	}
	return abs, nil
}

// ProxyCacheZone is the nginx keys_zone for a cache directory.
// The shared default path keeps the name siroc_cache.
func ProxyCacheZone(cachePath string) string {
	if cachePath == "" || cachePath == DefaultProxyCachePath {
		return DefaultProxyCacheZone
	}
	sum := sha256.Sum256([]byte(cachePath))
	return "siroc_c_" + hex.EncodeToString(sum[:8])
}

// ProxyCache reports the cache directory and zone when this site caches proxy responses.
func ProxyCache(o Options) (cachePath, zone string, on bool) {
	if o.Proxy == nil || !o.Proxy.Cache {
		return "", "", false
	}
	cachePath = o.Proxy.CachePath
	if cachePath == "" {
		cachePath = DefaultProxyCachePath
	}
	return cachePath, ProxyCacheZone(cachePath), true
}

func parseProxyTarget(target string) (*url.URL, error) {
	if strings.ContainsAny(target, " \t\r\n;{}'\"`$\\") {
		return nil, fmt.Errorf("invalid proxy target")
	}
	u, err := url.Parse(target)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("invalid proxy target")
	}
	return u, nil
}

// NginxProxyLocation is the server-context location block for a reverse-proxy site.
// fallback is the stored proxy URL when the proxy form has not been saved yet.
func NginxProxyLocation(o Options, fallback string) (string, error) {
	if o.Proxy == nil {
		target := strings.TrimSpace(fallback)
		if target == "" {
			return "", nil
		}
		if _, err := parseProxyTarget(target); err != nil {
			return "", err
		}
		return legacyProxy(target), nil
	}
	p := o.Proxy
	target := strings.TrimSpace(p.Target)
	if target == "" {
		target = strings.TrimSpace(fallback)
	}
	u, err := parseProxyTarget(target)
	if err != nil {
		return "", err
	}
	loc := "/"
	if p.ShowPath && p.Path != "" && p.Path != "/" {
		loc = p.Path
	}
	host := p.Host
	if host == "" {
		host = u.Hostname()
	}
	var b strings.Builder
	if loc == "/" {
		b.WriteString("    location / {\n")
	} else {
		fmt.Fprintf(&b, "    location ^~ %s {\n", loc)
	}
	for _, ip := range p.Black {
		fmt.Fprintf(&b, "        deny %s;\n", ip)
	}
	if len(p.White) > 0 {
		for _, ip := range p.White {
			fmt.Fprintf(&b, "        allow %s;\n", ip)
		}
		b.WriteString("        deny all;\n")
	}
	for _, rw := range p.Rewrites {
		fmt.Fprintf(&b, "        rewrite ^%s(.*)$ %s$1 break;\n", trimSlash(rw.From), trimSlash(rw.To))
		fmt.Fprintf(&b, "        proxy_redirect %s %s;\n", rw.From, rw.To)
	}
	fmt.Fprintf(&b, "        proxy_pass %s;\n", target)
	b.WriteString("        proxy_http_version 1.1;\n")
	fmt.Fprintf(&b, "        proxy_set_header Host %s;\n", host)
	b.WriteString("        proxy_set_header X-Real-IP $remote_addr;\n")
	b.WriteString("        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n")
	b.WriteString("        proxy_set_header X-Forwarded-Proto $scheme;\n")
	if u.Scheme == "https" {
		b.WriteString("        proxy_ssl_server_name on;\n")
		fmt.Fprintf(&b, "        proxy_ssl_name %s;\n", u.Hostname())
	}
	if p.Websocket {
		b.WriteString("        proxy_set_header Upgrade $http_upgrade;\n")
		b.WriteString("        proxy_set_header Connection $connection_upgrade;\n")
	}
	fmt.Fprintf(&b, "        proxy_connect_timeout %ds;\n", p.ConnectTimeout)
	fmt.Fprintf(&b, "        proxy_send_timeout %ds;\n", p.SendTimeout)
	fmt.Fprintf(&b, "        proxy_read_timeout %ds;\n", p.ReadTimeout)
	if p.Cache {
		cachePath := p.CachePath
		if cachePath == "" {
			cachePath = DefaultProxyCachePath
		}
		fmt.Fprintf(&b, "        proxy_cache %s;\n", ProxyCacheZone(cachePath))
		b.WriteString("        proxy_cache_key $scheme$host$request_uri;\n")
		b.WriteString("        proxy_cache_valid 200 301 302 1h;\n")
		b.WriteString("        proxy_ignore_headers Set-Cookie Cache-Control Expires X-Accel-Expires;\n")
		b.WriteString("        proxy_cache_bypass $http_authorization;\n")
	}
	if p.Gzip {
		b.WriteString("        gzip on;\n")
		b.WriteString("        gzip_min_length 1k;\n")
		b.WriteString("        gzip_comp_level 5;\n")
		b.WriteString("        gzip_types text/plain text/css application/json application/javascript text/xml application/xml image/svg+xml;\n")
	}
	if len(p.Replacements) > 0 {
		b.WriteString("        proxy_set_header Accept-Encoding \"\";\n")
		once := true
		for _, rp := range p.Replacements {
			if strings.Contains(rp.Rule, "g") {
				once = false
			}
			fmt.Fprintf(&b, "        sub_filter '%s' '%s';\n", rp.From, rp.To)
		}
		if once {
			b.WriteString("        sub_filter_once on;\n")
		} else {
			b.WriteString("        sub_filter_once off;\n")
		}
	}
	if p.Config != "" {
		for _, line := range strings.Split(p.Config, "\n") {
			fmt.Fprintf(&b, "        %s\n", line)
		}
	}
	b.WriteString("    }\n")
	return b.String(), nil
}

func legacyProxy(target string) string {
	return "    location / {\n" +
		"        proxy_pass " + target + ";\n" +
		"        proxy_http_version 1.1;\n" +
		"        proxy_set_header Host $http_host;\n" +
		"        proxy_set_header X-Real-IP $remote_addr;\n" +
		"        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n" +
		"        proxy_set_header X-Forwarded-Proto $scheme;\n" +
		"        proxy_set_header Upgrade $http_upgrade;\n" +
		"        proxy_set_header Connection \"upgrade\";\n" +
		"        proxy_read_timeout 300s;\n" +
		"    }\n"
}

func trimSlash(p string) string {
	if p == "/" {
		return ""
	}
	return strings.TrimSuffix(p, "/")
}

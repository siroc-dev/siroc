package siteopts

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Options are per-site nginx/apache settings edited from the site dialog.
type Options struct {
	Index       []string   `json:"index,omitempty"`
	Access      string     `json:"access,omitempty"`
	IPs         []string   `json:"ips,omitempty"`
	Hotlink     bool       `json:"hotlink,omitempty"`
	Maintenance bool       `json:"maintenance,omitempty"`
	Redirects   []Redirect `json:"redirects,omitempty"`
	Proxy       *Proxy     `json:"proxy,omitempty"`
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
	return len(o.Index) == 0 && o.Access == "" && len(o.IPs) == 0 && !o.Hotlink && !o.Maintenance && len(o.Redirects) == 0 && o.Proxy == nil
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
	return out, nil
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
		b.WriteString("        valid_referers none blocked server_names;\n")
		b.WriteString("        if ($invalid_referer) {\n")
		b.WriteString("            return 403;\n")
		b.WriteString("        }\n")
		fmt.Fprintf(&b, "        proxy_pass %s;\n", backend)
		b.WriteString("        proxy_set_header Host $http_host;\n")
		b.WriteString("        proxy_set_header X-Real-IP $remote_addr;\n")
		b.WriteString("        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n")
		b.WriteString("        proxy_set_header X-Forwarded-Proto $scheme;\n")
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

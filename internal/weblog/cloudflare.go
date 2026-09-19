//go:build linux

package weblog

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	realIPPath   = "/etc/nginx/conf.d/cp-realip.conf"
	cfMetaPath   = "/var/lib/siroc/cloudflare-ips.meta"
	cfV4URL      = "https://www.cloudflare.com/ips-v4"
	cfV6URL      = "https://www.cloudflare.com/ips-v6"
	realIPModule = "http_realip_module"
)

var fallbackCloudflare = []string{
	"173.245.48.0/20",
	"103.21.244.0/22",
	"103.22.200.0/22",
	"103.31.4.0/22",
	"141.101.64.0/18",
	"108.162.192.0/18",
	"190.93.240.0/20",
	"188.114.96.0/20",
	"197.234.240.0/22",
	"198.41.128.0/17",
	"162.158.0.0/15",
	"104.16.0.0/13",
	"104.24.0.0/14",
	"172.64.0.0/13",
	"131.0.72.0/22",
	"2400:cb00::/32",
	"2606:4700::/32",
	"2803:f800::/32",
	"2405:b500::/32",
	"2405:8100::/32",
	"2a06:98c0::/29",
	"2c0f:f248::/32",
}

var trustedProxies = []string{
	"127.0.0.1/32",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"::1/128",
	"fc00::/7",
	"fe80::/10",
}

func UpdateCloudflare(forceReload bool) error {
	if _, err := os.Stat("/usr/sbin/nginx"); err != nil {
		return fmt.Errorf("nginx is not installed")
	}
	cidrs, source, err := fetchCloudflare()
	if err != nil {
		cidrs = append([]string{}, fallbackCloudflare...)
		source = "fallback"
		if existing := readCIDRsFromConf(); len(existing) > 0 {
			cidrs = existing
			source = "cached"
		}
		if forceReload && source == "fallback" {
			// still write fallback so a first-time install works offline
		} else if err != nil && source == "cached" {
			_ = writeMeta(len(cidrs), source+" ("+err.Error()+")")
			if forceReload {
				return reloadNginx()
			}
			return nil
		}
	}
	body := renderRealIP(cidrs)
	old, _ := os.ReadFile(realIPPath)
	if err := os.MkdirAll("/etc/nginx/conf.d", 0755); err != nil {
		return err
	}
	if err := os.WriteFile(realIPPath, []byte(body), 0644); err != nil {
		return err
	}
	if err := testNginx(); err != nil {
		if len(old) > 0 {
			_ = os.WriteFile(realIPPath, old, 0644)
		} else {
			_ = os.Remove(realIPPath)
		}
		return err
	}
	_ = writeMeta(len(cidrs), source)
	if forceReload || string(old) != body {
		return reloadNginx()
	}
	return nil
}

func fetchCloudflare() ([]string, string, error) {
	v4, err := fetchCIDRs(cfV4URL)
	if err != nil {
		return nil, "", err
	}
	v6, err := fetchCIDRs(cfV6URL)
	if err != nil {
		return nil, "", err
	}
	if len(v4) < 5 {
		return nil, "", fmt.Errorf("cloudflare IPv4 list too small (%d)", len(v4))
	}
	return append(v4, v6...), "cloudflare", nil
}

func fetchCIDRs(url string) ([]string, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "siroc/1.0")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", url, res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, _, err := net.ParseCIDR(line); err != nil {
			if ip := net.ParseIP(line); ip != nil {
				if ip.To4() != nil {
					line += "/32"
				} else {
					line += "/128"
				}
			} else {
				continue
			}
		}
		out = append(out, line)
	}
	return out, nil
}

func renderRealIP(cidrs []string) string {
	var b strings.Builder
	b.WriteString("# Managed by siroc. Do not edit.\n")
	b.WriteString("# Cloudflare visitor IP via ngx_http_realip_module, refreshed daily.\n")
	b.WriteString("real_ip_header CF-Connecting-IP;\n")
	b.WriteString("real_ip_recursive on;\n\n")
	b.WriteString("# Local / Docker proxies so Cloudflare headers are trusted in containers.\n")
	for _, c := range trustedProxies {
		fmt.Fprintf(&b, "set_real_ip_from %s;\n", c)
	}
	b.WriteString("\n# Cloudflare anycast ranges\n")
	seen := map[string]struct{}{}
	for _, c := range cidrs {
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		fmt.Fprintf(&b, "set_real_ip_from %s;\n", c)
	}
	return b.String()
}

func readCIDRsFromConf() []string {
	b, err := os.ReadFile(realIPPath)
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "set_real_ip_from ") {
			continue
		}
		c := strings.Trim(strings.TrimPrefix(line, "set_real_ip_from "), "; ")
		skip := false
		for _, t := range trustedProxies {
			if c == t {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		if _, _, err := net.ParseCIDR(c); err == nil {
			out = append(out, c)
		}
	}
	return out
}

func writeMeta(prefixes int, source string) error {
	_ = os.MkdirAll("/var/lib/siroc", 0750)
	body := fmt.Sprintf("%s\n%d\n%s\n", time.Now().UTC().Format(time.RFC3339), prefixes, source)
	return os.WriteFile(cfMetaPath, []byte(body), 0644)
}

func cloudflareStatus() (prefixes int, updated, source string) {
	b, err := os.ReadFile(cfMetaPath)
	if err != nil {
		cidrs := readCIDRsFromConf()
		return len(cidrs), "", ""
	}
	parts := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(parts) > 0 {
		updated = parts[0]
	}
	if len(parts) > 1 {
		fmt.Sscanf(parts[1], "%d", &prefixes)
	}
	if len(parts) > 2 {
		source = parts[2]
	}
	if prefixes == 0 {
		prefixes = len(readCIDRsFromConf())
	}
	return
}

func nginxHasRealIP() bool {
	out, err := exec.Command("nginx", "-V").CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), realIPModule)
}

func testNginx() error {
	out, err := exec.Command("nginx", "-t").CombinedOutput()
	if err != nil {
		return fmt.Errorf("nginx -t: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func reloadNginx() error {
	if err := testNginx(); err != nil {
		return err
	}
	if exec.Command("systemctl", "is-active", "--quiet", "nginx").Run() != nil {
		return nil
	}
	if err := exec.Command("systemctl", "reload", "nginx").Run(); err != nil {
		return fmt.Errorf("reload nginx: %w", err)
	}
	return nil
}

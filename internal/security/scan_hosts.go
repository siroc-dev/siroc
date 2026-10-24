//go:build linux

package security

import (
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	hostsFile     = "/etc/hosts"
	hostsBegin    = "# BEGIN SIROC"
	hostsEnd      = "# END SIROC"
	hostsBlockReS = `(?ms)\n?# BEGIN (?:SIROC|CP-SERVER)\n.*?# END (?:SIROC|CP-SERVER)\n?`
)

var serverNameRe = regexp.MustCompile(`(?m)^\s*server_name\s+([^;]+);`)

// SyncLocalVhostHosts writes local-only nginx names into /etc/hosts so a scanner
// on this machine can open them. Public names are left to DNS. Pinning those to
// 127.0.0.1 makes a reverse proxy call this server, which sends the request
// through Cloudflare a second time.
func SyncLocalVhostHosts(extra ...string) {
	names := map[string]struct{}{}
	for _, g := range []string{
		"/etc/nginx/sites-enabled/*.conf",
		"/etc/nginx/sites-available/*.conf",
	} {
		paths, _ := filepath.Glob(g)
		for _, p := range paths {
			b, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			for _, m := range serverNameRe.FindAllStringSubmatch(string(b), -1) {
				for _, n := range strings.Fields(m[1]) {
					n = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(n), "."))
					if !hostNameOK(n) || !keepScanHost(n) {
						continue
					}
					names[n] = struct{}{}
				}
			}
		}
	}
	for _, n := range extra {
		n = strings.ToLower(strings.TrimSpace(n))
		if hostNameOK(n) && keepScanHost(n) {
			names[n] = struct{}{}
		}
	}
	list := make([]string, 0, len(names))
	for n := range names {
		list = append(list, n)
	}
	sort.Strings(list)
	writeHostsBlock(list)
}

func pinScanHost(target string) {
	u, err := url.Parse(target)
	if err != nil {
		SyncLocalVhostHosts()
		return
	}
	host := strings.ToLower(u.Hostname())
	if host != "" && localScanTLD(host) {
		SyncLocalVhostHosts(host)
		return
	}
	SyncLocalVhostHosts()
}

func hostNameOK(n string) bool {
	if n == "" || n == "_" || n == "localhost" || strings.Contains(n, "*") {
		return false
	}
	if net.ParseIP(n) != nil {
		return false
	}
	if strings.ContainsAny(n, " \t\n/#") {
		return false
	}
	return true
}

func writeHostsBlock(names []string) {
	b, err := os.ReadFile(hostsFile)
	if err != nil && !os.IsNotExist(err) {
		return
	}
	text := string(b)
	text = regexp.MustCompile(hostsBlockReS).ReplaceAllString(text, "\n")
	text = strings.TrimRight(text, "\n") + "\n"
	if len(names) == 0 {
		_ = os.WriteFile(hostsFile, []byte(text), 0644)
		return
	}
	block := hostsBegin + "\n127.0.0.1\t" + strings.Join(names, " ") + "\n" + hostsEnd + "\n"
	_ = os.WriteFile(hostsFile, []byte(text+block), 0644)
}

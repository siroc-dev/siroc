package security

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

const maxFail2banIgnore = 64

// NormalizeIgnore checks whitelist addresses and returns them in canonical form.
// A host and the same host as /32 or /128 collapse to one entry. Localhost is rejected
// because the jail file always ignores it.
func NormalizeIgnore(lines []string) ([]string, error) {
	seen := map[string]struct{}{}
	var out []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.ContainsAny(line, " \t\r\n'\"`;$\\") {
			return nil, fmt.Errorf("invalid address %q", line)
		}
		norm, err := normalizeIgnore(line)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[norm]; ok {
			continue
		}
		seen[norm] = struct{}{}
		out = append(out, norm)
	}
	if len(out) > maxFail2banIgnore {
		return nil, fmt.Errorf("whitelist is limited to %d addresses", maxFail2banIgnore)
	}
	sort.Strings(out)
	return out, nil
}

func normalizeIgnore(s string) (string, error) {
	if strings.Contains(s, "/") {
		ip, n, err := net.ParseCIDR(s)
		if err != nil || ip == nil || n == nil {
			return "", fmt.Errorf("invalid network %s", s)
		}
		ones, bits := n.Mask.Size()
		if ones == 0 || (bits == 32 && ones < 8) || (bits == 128 && ones < 16) {
			return "", fmt.Errorf("network %s is too wide", n.String())
		}
		if ones == bits {
			return hostIgnore(n.IP)
		}
		if ipv4Net(n) && loopback4.Contains(n.IP) {
			return "", fmt.Errorf("%s is already ignored as localhost", n.String())
		}
		if !ipv4Net(n) && n.IP.IsLoopback() {
			return "", fmt.Errorf("%s is already ignored as localhost", n.String())
		}
		return n.String(), nil
	}
	ip := net.ParseIP(s)
	if ip == nil {
		return "", fmt.Errorf("invalid address %s", s)
	}
	return hostIgnore(ip)
}

func hostIgnore(ip net.IP) (string, error) {
	if ip.IsLoopback() || ip.IsUnspecified() {
		return "", fmt.Errorf("%s is already ignored as localhost", ip.String())
	}
	return ip.String(), nil
}

var loopback4 = &net.IPNet{IP: net.IPv4(127, 0, 0, 0).To4(), Mask: net.CIDRMask(8, 32)}

func ipv4Net(n *net.IPNet) bool {
	return n != nil && n.IP.To4() != nil && len(n.Mask) == net.IPv4len
}

// RenderFail2banIgnore writes jail.d/siroc-ignore.local. Localhost is always included.
func RenderFail2banIgnore(addrs []string) (string, error) {
	norm, err := NormalizeIgnore(addrs)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("# Siroc Fail2ban whitelist. Localhost is always ignored.\n")
	b.WriteString("[DEFAULT]\nignoreip = 127.0.0.1/8 ::1")
	for _, a := range norm {
		b.WriteByte(' ')
		b.WriteString(a)
	}
	b.WriteByte('\n')
	return b.String(), nil
}

// IPIgnored reports whether a banned address is covered by the whitelist.
func IPIgnored(ip string, addrs []string) bool {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return false
	}
	if parsed.IsLoopback() {
		return true
	}
	for _, a := range addrs {
		if strings.Contains(a, "/") {
			_, n, err := net.ParseCIDR(a)
			if err == nil && n.Contains(parsed) {
				return true
			}
			continue
		}
		if parsed.Equal(net.ParseIP(a)) {
			return true
		}
	}
	return false
}

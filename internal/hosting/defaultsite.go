package hosting

import (
	"fmt"
	"strings"
)

// SetListenDefault adds or removes nginx's default_server flag on public listen lines.
// A loopback listener is left unchanged. Only the first listener for an address keeps the flag.
func SetListenDefault(conf string, on bool) string {
	lines := strings.Split(conf, "\n")
	seen := map[string]bool{}
	for i, line := range lines {
		addr, ok := listenAddress(line)
		if !ok || !publicListen(addr) {
			continue
		}
		add := on
		if on {
			key := listenKey(addr)
			add = !seen[key]
			seen[key] = true
		}
		next, changed := rewriteListenDefault(line, add)
		if changed {
			lines[i] = next
		}
	}
	return strings.Join(lines, "\n")
}

func listenAddress(line string) (string, bool) {
	trim := strings.TrimSpace(line)
	if !strings.HasPrefix(trim, "listen ") || !strings.HasSuffix(trim, ";") {
		return "", false
	}
	args := strings.Fields(strings.TrimSuffix(trim, ";"))
	if len(args) < 2 {
		return "", false
	}
	return args[1], true
}

func listenKey(addr string) string {
	if strings.HasPrefix(addr, "[") {
		return addr
	}
	host, port := "*", addr
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		host, port = addr[:i], addr[i+1:]
	}
	if host == "" || host == "*" || host == "0.0.0.0" {
		host = "*"
	}
	return host + ":" + port
}

func rewriteListenDefault(line string, on bool) (string, bool) {
	trim := strings.TrimSpace(line)
	if !strings.HasPrefix(trim, "listen ") || !strings.HasSuffix(trim, ";") {
		return line, false
	}
	indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	body := strings.TrimSpace(strings.TrimSuffix(trim, ";"))
	args := strings.Fields(body)
	if len(args) < 2 {
		return line, false
	}
	addr := args[1]
	if !publicListen(addr) {
		return line, false
	}
	kept := make([]string, 0, len(args))
	for _, arg := range args[1:] {
		if arg != "default_server" {
			kept = append(kept, arg)
		}
	}
	if on && publicListen(addr) {
		kept = append(kept, "default_server")
	}
	return indent + "listen " + strings.Join(kept, " ") + ";", true
}

func publicListen(addr string) bool {
	switch {
	case strings.HasPrefix(addr, "unix:"),
		strings.HasPrefix(addr, "127."),
		strings.HasPrefix(addr, "[::1]"),
		strings.HasPrefix(addr, "localhost"):
		return false
	default:
		return true
	}
}

func cleanDefaultDomain(domain string) (string, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return "", nil
	}
	if strings.Contains(domain, "..") || strings.ContainsAny(domain, "/\\ \t") {
		return "", fmt.Errorf("invalid website")
	}
	for _, r := range domain {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '.' && r != '-' {
			return "", fmt.Errorf("invalid website")
		}
	}
	return domain, nil
}

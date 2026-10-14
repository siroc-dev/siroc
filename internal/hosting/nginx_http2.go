package hosting

import "strings"

// nginxVersionUsesHTTP2Directive reports whether this nginx wants "http2 on"
// instead of "listen ... http2". The separate directive arrived in 1.25.1.
func nginxVersionUsesHTTP2Directive(raw string) bool {
	ver := strings.TrimSpace(raw)
	if i := strings.LastIndex(ver, "/"); i >= 0 {
		ver = ver[i+1:]
	}
	ver = strings.TrimSpace(ver)
	major, minor, patch, ok := cutNginxVersion(ver)
	if !ok {
		return false
	}
	if major != 1 {
		return major > 1
	}
	if minor != 25 {
		return minor > 25
	}
	return patch >= 1
}

func cutNginxVersion(s string) (major, minor, patch int, ok bool) {
	parts := strings.SplitN(s, ".", 3)
	if len(parts) < 2 {
		return 0, 0, 0, false
	}
	major, ok1 := atoiPrefix(parts[0])
	minor, ok2 := atoiPrefix(parts[1])
	patch = 0
	ok3 := true
	if len(parts) == 3 {
		patch, ok3 = atoiPrefix(parts[2])
	}
	if !ok1 || !ok2 || !ok3 {
		return 0, 0, 0, false
	}
	return major, minor, patch, true
}

func atoiPrefix(s string) (int, bool) {
	n := 0
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		n = n*10 + int(s[i]-'0')
		i++
	}
	if i == 0 {
		return 0, false
	}
	return n, true
}

// rewriteListenHTTP2 turns "listen ... http2" into a listen line plus "http2 on".
// A file that already uses the new directive is left unchanged.
func rewriteListenHTTP2(body string) (string, bool) {
	if !strings.Contains(body, "listen") || !strings.Contains(body, "http2") {
		return body, false
	}
	lines := strings.Split(body, "\n")
	changed := false
	var out []string
	for i := 0; i < len(lines); {
		next, ok := stripListenHTTP2(lines[i])
		if !ok {
			out = append(out, lines[i])
			i++
			continue
		}
		changed = true
		indent := leadingSpace(lines[i])
		out = append(out, next)
		i++
		for i < len(lines) {
			nxt, ok := stripListenHTTP2(lines[i])
			if !ok {
				break
			}
			out = append(out, nxt)
			i++
		}
		if !upcomingHTTP2(lines[i:]) {
			out = append(out, indent+"http2 on;")
		}
	}
	if !changed {
		return body, false
	}
	return strings.Join(out, "\n"), true
}

func stripListenHTTP2(line string) (string, bool) {
	raw := line
	comment := ""
	if i := strings.Index(line, "#"); i >= 0 {
		comment = line[i:]
		line = line[:i]
	}
	trim := strings.TrimSpace(line)
	if !strings.HasPrefix(trim, "listen") || !strings.Contains(trim, "http2") {
		return raw, false
	}
	parts := strings.Fields(trim)
	kept := make([]string, 0, len(parts))
	found := false
	for _, p := range parts {
		semi := strings.HasSuffix(p, ";")
		p = strings.TrimSuffix(p, ";")
		if p == "http2" {
			found = true
			continue
		}
		if semi {
			p += ";"
		}
		kept = append(kept, p)
	}
	if !found {
		return raw, false
	}
	body := strings.Join(kept, " ")
	if !strings.HasSuffix(body, ";") {
		body += ";"
	}
	return leadingSpace(raw) + body + comment, true
}

func upcomingHTTP2(lines []string) bool {
	depth := 0
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "http2 ") {
			return true
		}
		if strings.HasPrefix(trim, "location") || strings.HasPrefix(trim, "server") {
			return false
		}
		depth += strings.Count(line, "{") - strings.Count(line, "}")
		if depth < 0 {
			return false
		}
	}
	return false
}

func leadingSpace(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

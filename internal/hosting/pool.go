package hosting

import "strings"

// setPoolDirective sets one bare pool key, such as chdir, and drops duplicates.
func setPoolDirective(content, key, value string) string {
	line := key + " = " + value
	var b strings.Builder
	found := false
	for _, raw := range strings.SplitAfter(content, "\n") {
		if poolDirectiveKey(raw) == key {
			if !found {
				b.WriteString(line + "\n")
				found = true
			}
			continue
		}
		b.WriteString(raw)
	}
	out := b.String()
	if !found {
		if out != "" && !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		out += line + "\n"
	}
	return out
}

func poolDirectiveKey(raw string) string {
	trim := strings.TrimSpace(raw)
	if trim == "" || strings.HasPrefix(trim, ";") || strings.HasPrefix(trim, "#") {
		return ""
	}
	parts := strings.SplitN(trim, "=", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

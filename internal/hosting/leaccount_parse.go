package hosting

import "strings"

func parseCertbotAccount(out string) (uri, email string, ok bool) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if v, found := colonField(line, "account url"); found {
			uri = v
			continue
		}
		if v, found := colonField(line, "email contact"); found {
			email = strings.Trim(v, "[]")
			continue
		}
	}
	return uri, email, uri != ""
}

func certbotAccountExists(out string) bool {
	s := strings.ToLower(out)
	return strings.Contains(s, "existing account") ||
		strings.Contains(s, "already exists") ||
		strings.Contains(s, "already registered")
}

func colonField(line, key string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(line))
	prefix := strings.ToLower(key) + ":"
	if !strings.HasPrefix(lower, prefix) {
		return "", false
	}
	i := strings.Index(line, ":")
	if i < 0 {
		return "", false
	}
	return strings.TrimSpace(line[i+1:]), true
}

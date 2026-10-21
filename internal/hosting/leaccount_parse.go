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

func certbotBusy(out string) bool {
	return strings.Contains(out, "Another instance of Certbot is already running")
}

// describeCertbotBusy names the website the other Certbot process is handling.
// cmdlines are process arguments. logTail is the end of letsencrypt.log.
func describeCertbotBusy(cmdlines []string, logTail string) string {
	names := certbotCmdNames(cmdlines)
	if len(names) == 0 {
		names = certbotLogNames(logTail)
	}
	if len(names) > 0 {
		return "Certbot is working on " + strings.Join(names, ", ") + "."
	}
	switch certbotCmdAction(cmdlines) {
	case "renew":
		return "Certbot is renewing certificates."
	case "":
		return ""
	default:
		return "Certbot is running " + certbotCmdAction(cmdlines) + "."
	}
}

func certbotCmdNames(cmdlines []string) []string {
	var names []string
	seen := map[string]struct{}{}
	add := func(name string) {
		name = strings.Trim(strings.TrimSpace(name), `"'`)
		if !certbotNameOK(name) {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	for _, line := range cmdlines {
		if !strings.Contains(line, "certbot") {
			continue
		}
		fields := strings.Fields(line)
		for i := 0; i < len(fields); i++ {
			f := fields[i]
			switch {
			case f == "-d" || f == "--domain" || f == "--domains" || f == "--cert-name":
				if i+1 < len(fields) {
					add(fields[i+1])
					i++
				}
			case strings.HasPrefix(f, "--cert-name="):
				add(strings.TrimPrefix(f, "--cert-name="))
			case strings.HasPrefix(f, "--domains="), strings.HasPrefix(f, "--domain="):
				add(f[strings.IndexByte(f, '=')+1:])
			}
		}
	}
	return names
}

func certbotCmdAction(cmdlines []string) string {
	for _, line := range cmdlines {
		if !strings.Contains(line, "certbot") {
			continue
		}
		for _, f := range strings.Fields(line) {
			switch f {
			case "renew", "certonly", "register", "update_account", "show_account", "certificates", "delete", "revoke":
				return f
			}
		}
	}
	return ""
}

func certbotLogNames(logTail string) []string {
	var names []string
	seen := map[string]struct{}{}
	add := func(name string) {
		name = strings.Trim(strings.TrimSpace(name), `"'`)
		if !certbotNameOK(name) {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	var last []string
	for _, line := range strings.Split(logTail, "\n") {
		line = strings.TrimSpace(line)
		if i := strings.Index(line, "Processing /etc/letsencrypt/renewal/"); i >= 0 {
			rest := line[i+len("Processing /etc/letsencrypt/renewal/"):]
			if j := strings.Index(rest, ".conf"); j > 0 {
				rest = rest[:j]
			}
			if certbotNameOK(rest) {
				last = []string{rest}
			}
			continue
		}
		for _, prefix := range []string{"Requesting a certificate for ", "Renewing an existing certificate for "} {
			if i := strings.Index(line, prefix); i >= 0 {
				last = splitCertbotNames(line[i+len(prefix):])
			}
		}
	}
	for _, name := range last {
		add(name)
	}
	return names
}

func splitCertbotNames(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, ".")
	parts := strings.Split(s, " and ")
	var out []string
	for _, p := range parts {
		p = strings.Trim(strings.TrimSpace(p), ",")
		if certbotNameOK(p) {
			out = append(out, p)
		}
	}
	return out
}

func certbotNameOK(name string) bool {
	if name == "" || strings.HasPrefix(name, "-") || len(name) > 253 {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '-' || r == '_' || r == '*':
		default:
			return false
		}
	}
	return true
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

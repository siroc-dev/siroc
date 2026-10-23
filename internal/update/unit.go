package update

import "strings"

// StripRuntimeDirectoryGroup drops a unit key that systemd before 256 rejects.
// The agent sets the runtime directory group itself.
func StripRuntimeDirectoryGroup(text string) (string, bool) {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	changed := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "RuntimeDirectoryGroup=") {
			changed = true
			continue
		}
		out = append(out, line)
	}
	if !changed {
		return text, false
	}
	return strings.Join(out, "\n"), true
}

// AfterCloudInit runs the agent after cloud-init, which rewrites /etc/hosts
// during boot. The agent then puts the saved Hosts rows back.
func AfterCloudInit(text string) (string, bool) {
	const want = "cloud-init.service"
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if !strings.HasPrefix(trim, "After=") {
			continue
		}
		for _, field := range strings.Fields(strings.TrimPrefix(trim, "After=")) {
			if field == want {
				return text, false
			}
		}
		lines[i] = strings.TrimRight(line, " \t") + " " + want
		return strings.Join(lines, "\n"), true
	}
	var out []string
	inserted := false
	for _, line := range lines {
		out = append(out, line)
		if !inserted && strings.TrimSpace(line) == "[Unit]" {
			out = append(out, "After="+want)
			inserted = true
		}
	}
	if !inserted {
		return text, false
	}
	return strings.Join(out, "\n"), true
}

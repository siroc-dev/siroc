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

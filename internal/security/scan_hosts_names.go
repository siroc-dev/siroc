package security

import "strings"

// keepScanHost reports names that only exist on this machine. Public names stay
// on real DNS, so a reverse proxy does not connect back to 127.0.0.1.
func keepScanHost(n string) bool {
	n = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(n), "."))
	if n == "pma.cp.local" {
		return true
	}
	return localScanTLD(n)
}

func localScanTLD(host string) bool {
	for _, sfx := range []string{".test", ".localhost", ".invalid", ".example", ".local", ".lan", ".internal", ".home", ".corp", ".private", ".localdomain"} {
		if strings.HasSuffix(host, sfx) {
			return true
		}
	}
	return false
}

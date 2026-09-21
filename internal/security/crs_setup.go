package security

import (
	"os"
	"strings"
)

func parseCRSSetupVersion(text string) int {
	needle := "tx.crs_setup_version="
	for _, line := range strings.Split(text, "\n") {
		low := strings.ToLower(line)
		i := strings.Index(low, needle)
		if i < 0 {
			continue
		}
		rest := line[i+len(needle):]
		n := 0
		for _, c := range rest {
			if c < '0' || c > '9' {
				break
			}
			n = n*10 + int(c-'0')
		}
		if n > 0 {
			return n
		}
	}
	return 0
}

func crsSetupVersionFromFiles(paths ...string) int {
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if v := parseCRSSetupVersion(string(b)); v > 0 {
			return v
		}
	}
	return 0
}

func defaultCRSSetupVersion() int {
	if v := crsSetupVersionFromFiles(
		"/etc/modsecurity/crs/crs-setup.conf",
		"/usr/share/modsecurity-crs/crs-setup.conf.example",
		"/usr/share/modsecurity-crs/owasp-crs/crs-setup.conf.example",
		"/etc/modsecurity/crs/crs-setup.conf.example",
		"/usr/share/modsecurity-crs/crs-setup.conf",
	); v > 0 {
		return v
	}
	return 335
}

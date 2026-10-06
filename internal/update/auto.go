package update

import (
	"os"
	"path/filepath"
	"strings"
)

// autoUpdateFile is on unless it contains off. A missing file stays on so
// existing servers keep the hourly install.
var autoUpdateFile = "/var/lib/siroc/auto-update"

func AutoEnabled() bool {
	b, err := os.ReadFile(autoUpdateFile)
	if err != nil {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(string(b))) {
	case "off", "0", "false", "no":
		return false
	default:
		return true
	}
}

func writeAuto(on bool) error {
	if err := os.MkdirAll(filepath.Dir(autoUpdateFile), 0755); err != nil {
		return err
	}
	v := "on"
	if !on {
		v = "off"
	}
	return os.WriteFile(autoUpdateFile, []byte(v+"\n"), 0644)
}

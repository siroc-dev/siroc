package backup

import (
	"fmt"
	"path"
	"strings"
)

// MaxRestoreUpload is the largest archive Restore will accept from the browser.
const MaxRestoreUpload = 10 << 30

func RestoreArchiveName(name string) (string, error) {
	name = strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	base := path.Base(name)
	if base == "." || base == "/" || base == "" || strings.Contains(base, "..") {
		return "", fmt.Errorf("invalid archive name")
	}
	lower := strings.ToLower(base)
	ext := ""
	switch {
	case strings.HasSuffix(lower, ".tar.gz"):
		ext = ".tar.gz"
	case strings.HasSuffix(lower, ".tar.zst"):
		ext = ".tar.zst"
	case strings.HasSuffix(lower, ".tgz"):
		ext = ".tgz"
	case strings.HasSuffix(lower, ".tzst"):
		ext = ".tzst"
	case strings.HasSuffix(lower, ".tar"):
		ext = ".tar"
	default:
		return "", fmt.Errorf("upload a .tar.gz, .tgz, .tar.zst, .tzst, or .tar archive")
	}
	var b strings.Builder
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	safe := b.String()
	if len(safe) > 120 {
		safe = safe[:80] + safe[len(safe)-len(ext):]
	}
	if !strings.HasSuffix(strings.ToLower(safe), ext) {
		return "", fmt.Errorf("invalid archive name")
	}
	return safe, nil
}

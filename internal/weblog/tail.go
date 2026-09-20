package weblog

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	defaultTailBytes = 256 * 1024
	maxTailBytes     = 1024 * 1024
)

var laravelLogNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,80}\.log$`)

func ClampTailBytes(n int) int {
	if n <= 0 {
		return defaultTailBytes
	}
	if n > maxTailBytes {
		return maxTailBytes
	}
	return n
}

func TailBytes(b []byte, max int) (out []byte, truncated bool) {
	max = ClampTailBytes(max)
	if len(b) <= max {
		return b, false
	}
	start := len(b) - max
	if i := bytes.IndexByte(b[start:], '\n'); i >= 0 && start+i+1 < len(b) {
		start += i + 1
	}
	return b[start:], true
}

func ValidLaravelLogName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}
	return laravelLogNameRe.MatchString(name)
}

func InHome(home, path string) bool {
	clean := filepath.Clean(path)
	home = filepath.Clean(home)
	return clean == home || strings.HasPrefix(clean, home+string(os.PathSeparator))
}

func ParseSiteLogID(id string) (group, name string, ok bool) {
	id = strings.TrimSpace(id)
	switch id {
	case "", "nginx-access":
		return "nginx", "access", true
	case "nginx-error":
		return "nginx", "error", true
	case "apache-access":
		return "apache", "access", true
	case "apache-error":
		return "apache", "error", true
	case "waf", "waf-audit":
		return "waf", "audit", true
	case "laravel":
		return "laravel", "laravel.log", true
	}
	if file, found := strings.CutPrefix(id, "laravel:"); found {
		if !ValidLaravelLogName(file) {
			return "", "", false
		}
		return "laravel", file, true
	}
	return "", "", false
}

package api

import (
	"os"
	"strings"
	"time"
	_ "time/tzdata"
)

func serverTimezone() string {
	if z := cleanTimezone(readTrimmed("/etc/timezone")); z != "" {
		return z
	}
	link, err := os.Readlink("/etc/localtime")
	if err != nil {
		return ""
	}
	const marker = "zoneinfo/"
	if i := strings.LastIndex(link, marker); i >= 0 {
		return cleanTimezone(link[i+len(marker):])
	}
	return ""
}

func readTrimmed(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func cleanTimezone(raw string) string {
	z := strings.TrimSpace(raw)
	if z == "" || strings.ContainsAny(z, "\r\n") {
		return ""
	}
	if _, err := time.LoadLocation(z); err != nil {
		return ""
	}
	return z
}

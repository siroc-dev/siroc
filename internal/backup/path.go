package backup

import "strings"

func RestorePathOK(src, username string) bool {
	src = strings.ReplaceAll(strings.TrimSpace(src), "\\", "/")
	if src == "" || strings.Contains(src, "..") || !strings.HasPrefix(src, "/") {
		return false
	}
	if strings.HasPrefix(src, "/var/backups/") {
		return true
	}
	user := strings.TrimSpace(username)
	if user != "" && strings.HasPrefix(src, "/home/"+user+"/") {
		return true
	}
	return false
}

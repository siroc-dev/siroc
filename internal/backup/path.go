package backup

import "strings"

func RestorePathOK(src, username string) bool {
	src = strings.ReplaceAll(strings.TrimSpace(src), "\\", "/")
	if src == "" || strings.Contains(src, "..") || !strings.HasPrefix(src, "/") {
		return false
	}
	if strings.HasPrefix(src, "/var/backups/") || strings.HasPrefix(src, "/home/tmp/") {
		return true
	}
	if sirocTmpPath(src) {
		return true
	}
	user := strings.TrimSpace(username)
	if user != "" && strings.HasPrefix(src, "/home/"+user+"/") {
		return true
	}
	return false
}

func sirocTmpPath(src string) bool {
	parts := strings.Split(src, "/")
	for i := 1; i < len(parts)-1; i++ {
		if parts[i] == "siroc-tmp" && parts[len(parts)-1] != "" {
			return true
		}
	}
	return false
}

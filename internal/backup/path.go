package backup

import (
	"fmt"
	"path"
	"strings"

	"github.com/siroc-dev/siroc/internal/validate"
)

// UserLocalDir is a backup directory inside /home/<username>.
// A relative path is joined onto that home. Anything else is rejected.
func UserLocalDir(username, raw string) (string, error) {
	if err := validate.LinuxUser(username); err != nil {
		return "", err
	}
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if raw == "" || strings.ContainsRune(raw, 0) {
		return "", fmt.Errorf("directory is required")
	}
	home := "/home/" + username
	clean := path.Clean(raw)
	if !strings.HasPrefix(raw, "/") {
		clean = path.Clean(home + "/" + raw)
	}
	if clean != home && !strings.HasPrefix(clean, home+"/") {
		return "", fmt.Errorf("local directory must stay inside %s", home)
	}
	return clean, nil
}

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

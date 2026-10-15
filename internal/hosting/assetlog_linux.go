//go:build linux

package hosting

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

func ensureAssetLogMap() error {
	if err := os.MkdirAll("/etc/nginx/conf.d", 0755); err != nil {
		return err
	}
	return os.WriteFile("/etc/nginx/conf.d/siroc-asset-log.conf", []byte(assetLogMap), 0644)
}

// RepairAssetLogs installs the asset map and stops existing site access logs
// from recording images, video, CSS, and JavaScript.
func RepairAssetLogs() {
	if err := ensureAssetLogMap(); err != nil {
		log.Printf("asset log map: %v", err)
		return
	}
	var edits []nginxEdit
	dir := "/etc/nginx/sites-available"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		next := quietAssetAccessLogs(string(b))
		if next == string(b) {
			continue
		}
		edits = append(edits, nginxEdit{path: path, prev: string(b), next: next, mode: info.Mode().Perm()})
	}
	if len(edits) == 0 {
		return
	}
	for _, edit := range edits {
		if err := os.WriteFile(edit.path, []byte(edit.next), edit.mode); err != nil {
			log.Printf("asset log: %s: %v", edit.path, err)
			restoreNginxEdits(edits)
			return
		}
	}
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		log.Printf("asset log: %s", string(out))
		restoreNginxEdits(edits)
		return
	}
	if out, err := exec.Command("systemctl", "reload", "nginx").CombinedOutput(); err != nil {
		log.Printf("nginx reload: %s", string(out))
	}
}

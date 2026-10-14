//go:build linux

package hosting

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func nginxSeparateHTTP2() bool {
	out, err := exec.Command("nginx", "-v").CombinedOutput()
	if err != nil && len(strings.TrimSpace(string(out))) == 0 {
		return false
	}
	return nginxVersionUsesHTTP2Directive(string(out))
}

// RepairNginxHTTP2 rewrites installed site files that still use "listen ... http2"
// on an nginx that wants the separate http2 directive. Older nginx is left as-is.
func RepairNginxHTTP2() {
	if !nginxSeparateHTTP2() {
		return
	}
	var edits []nginxEdit
	for _, dir := range []string{"/etc/nginx/sites-available", "/etc/nginx/sites-enabled", "/etc/nginx/conf.d"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
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
			next, ok := rewriteListenHTTP2(string(b))
			if !ok || next == string(b) {
				continue
			}
			edits = append(edits, nginxEdit{path: path, prev: string(b), next: next, mode: info.Mode().Perm()})
		}
	}
	if len(edits) == 0 {
		return
	}
	for _, edit := range edits {
		if err := os.WriteFile(edit.path, []byte(edit.next), edit.mode); err != nil {
			log.Printf("nginx http2: %s: %v", edit.path, err)
			restoreNginxEdits(edits)
			return
		}
	}
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		log.Printf("nginx http2: %s", strings.TrimSpace(string(out)))
		restoreNginxEdits(edits)
		return
	}
	if out, err := exec.Command("systemctl", "reload", "nginx").CombinedOutput(); err != nil {
		log.Printf("nginx reload: %s", strings.TrimSpace(string(out)))
	}
}

type nginxEdit struct {
	path string
	prev string
	next string
	mode os.FileMode
}

func restoreNginxEdits(edits []nginxEdit) {
	for _, edit := range edits {
		if err := os.WriteFile(edit.path, []byte(edit.prev), edit.mode); err != nil {
			log.Printf("nginx http2 restore: %s: %v", edit.path, err)
		}
	}
}

//go:build linux

package hosting

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const defaultSiteFile = "/var/lib/siroc/default-site"

func currentDefaultSite() string {
	b, err := os.ReadFile(defaultSiteFile)
	if err != nil {
		return ""
	}
	domain, err := cleanDefaultDomain(string(b))
	if err != nil {
		return ""
	}
	return domain
}

// ApplyDefaultWebsite makes one website the nginx default server.
// An empty domain restores the catch-all that closes unknown requests.
func ApplyDefaultWebsite(domain string) error {
	domain, err := cleanDefaultDomain(domain)
	if err != nil {
		return err
	}
	if domain != "" {
		if _, err := os.Stat(filepath.Join("/etc/nginx/sites-available", domain+".conf")); err != nil {
			return fmt.Errorf("website has no nginx config")
		}
	}
	entries, err := os.ReadDir("/etc/nginx/sites-available")
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var edits []nginxEdit
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".conf") {
			continue
		}
		path := filepath.Join("/etc/nginx/sites-available", entry.Name())
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		prev, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(entry.Name(), ".conf")
		on := domain != "" && name == domain
		if name == "siroc-default" {
			on = domain == ""
		}
		next := SetListenDefault(string(prev), on)
		if next == string(prev) {
			continue
		}
		edits = append(edits, nginxEdit{path: path, prev: string(prev), next: next, mode: info.Mode().Perm()})
	}
	prevMarker, markerErr := os.ReadFile(defaultSiteFile)
	if err := os.MkdirAll("/var/lib/siroc", 0755); err != nil {
		return err
	}
	if err := os.WriteFile(defaultSiteFile, []byte(domain+"\n"), 0644); err != nil {
		return err
	}
	for _, edit := range edits {
		if err := os.WriteFile(edit.path, []byte(edit.next), edit.mode); err != nil {
			restoreNginxEdits(edits)
			restoreDefaultMarker(prevMarker, markerErr)
			return err
		}
	}
	if err := testReload("nginx", "nginx", "-t"); err != nil {
		restoreNginxEdits(edits)
		restoreDefaultMarker(prevMarker, markerErr)
		return err
	}
	return nil
}

func restoreDefaultMarker(prev []byte, readErr error) {
	if readErr != nil {
		_ = os.Remove(defaultSiteFile)
		return
	}
	_ = os.WriteFile(defaultSiteFile, prev, 0644)
}

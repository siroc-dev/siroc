package files

import (
	"fmt"
	"path"
	"strings"

	"github.com/siroc-dev/siroc/internal/validate"
)

// candidatePath is the filesystem path a file-manager request should open.
// Paths are home-relative even when they start with a slash: "/domains/a"
// means "<home>/domains/a". An absolute site document root on another disk
// is the exception. Joining that onto the home produced
// /home/<user>/mnt/data/... which does not exist.
func candidatePath(home, rel string, roots []string) (string, error) {
	home = slashClean(home)
	if home == "" || home == "/" || home == "." {
		return "", fmt.Errorf("path is outside the account home")
	}
	rel = strings.TrimSpace(strings.ReplaceAll(rel, "\\", "/"))
	if rel == "" || rel == "/" || rel == "." {
		return home, nil
	}
	abs := rel
	if !strings.HasPrefix(abs, "/") {
		abs = "/" + abs
	}
	abs = path.Clean(abs)
	if matchedRoot(roots, abs) != "" && !within(home, abs) {
		return abs, nil
	}
	trimmed := strings.TrimPrefix(rel, "/")
	if trimmed == "" || trimmed == "." {
		return home, nil
	}
	joined := path.Clean(path.Join(home, trimmed))
	if joined != home && !within(home, joined) {
		return "", fmt.Errorf("path is outside the account home")
	}
	return joined, nil
}

// cleanSiteRoots keeps document roots that DocRoot would accept outside the account home.
func cleanSiteRoots(homeRoot, username string, roots []string) []string {
	homeRoot = slashClean(homeRoot)
	if homeRoot == "" || homeRoot == "/" {
		homeRoot = "/home"
	}
	home := path.Clean(homeRoot + "/" + username)
	var out []string
	seen := map[string]struct{}{}
	add := func(p string) {
		p = slashClean(p)
		if p == "" || p == home || within(home, p) {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	for _, raw := range roots {
		abs, err := validate.DocRoot(homeRoot, username, raw, "")
		if err != nil {
			continue
		}
		// Keep the stored path and, when it is a symlink, the resolved path.
		add(abs)
		add(raw)
		// A directory saved as /home/<user>/mnt/data/... is the mount /mnt/data/...,
		// not a folder under the home.
		if alt := mountAlias(home, abs); alt != "" {
			if cleaned, err := validate.DocRoot(homeRoot, username, alt, ""); err == nil {
				add(cleaned)
				add(alt)
			}
		}
	}
	return out
}

// mountAlias returns /mnt/... when a document root was stored under the account home
// by joining that absolute path onto the home.
func mountAlias(home, doc string) string {
	home = slashClean(home)
	doc = slashClean(doc)
	if home == "" || home == "/" || doc == "" {
		return ""
	}
	if doc != home && !strings.HasPrefix(doc, home+"/") {
		return ""
	}
	rel := strings.TrimPrefix(doc, home+"/")
	if !strings.HasPrefix(rel, "mnt/") {
		return ""
	}
	rest := strings.TrimPrefix(rel, "mnt/")
	if rest == "" || strings.Contains(rest, "..") {
		return ""
	}
	return "/" + rel
}

func pathAllowed(home, abs string, roots []string) bool {
	home = slashClean(home)
	abs = slashClean(abs)
	if abs == "" || abs == "/" || abs == "." {
		return false
	}
	if home != "" && home != "/" && home != "." && (abs == home || within(home, abs)) {
		return true
	}
	return matchedRoot(roots, abs) != ""
}

func matchedRoot(roots []string, abs string) string {
	var best string
	for _, root := range roots {
		root = slashClean(root)
		if within(root, abs) && len(root) > len(best) {
			best = root
		}
	}
	return best
}

func within(root, child string) bool {
	root = slashClean(root)
	child = slashClean(child)
	if root == "" || root == "/" || child == "" || child == "/" {
		return false
	}
	return child == root || strings.HasPrefix(child, root+"/")
}

// displayPath is the path the file manager shows and sends back.
// Inside the home it stays home-relative. A site folder on another disk stays absolute.
func displayPath(home, abs string) string {
	home = slashClean(home)
	abs = slashClean(abs)
	if abs == "" || abs == "." {
		return "/"
	}
	if home == "" || home == "/" || home == "." {
		return abs
	}
	if abs == home {
		return "/"
	}
	if within(home, abs) {
		return "/" + strings.TrimPrefix(abs, strings.TrimRight(home, "/")+"/")
	}
	return abs
}

func slashClean(p string) string {
	p = strings.TrimSpace(strings.ReplaceAll(p, "\\", "/"))
	if p == "" {
		return ""
	}
	abs := strings.HasPrefix(p, "/")
	p = path.Clean(p)
	if abs && !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if p == "." {
		return ""
	}
	return p
}

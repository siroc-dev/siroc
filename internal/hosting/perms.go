package hosting

import (
	"os"
	"path/filepath"
	"strings"
)

// chmodWebTree makes directories traversable and files readable by Apache.
// A symlink to a directory is followed when the target stays inside the account
// home. chmod on the link itself would follow it and could clear the directory
// execute bit, which makes Apache fail with AH00529 on .htaccess.
func chmodWebTree(home, root string) {
	home = filepath.Clean(home)
	root = filepath.Clean(root)
	if root != home && !withinHome(home, root) {
		return
	}
	seen := map[string]struct{}{}
	var walk func(string)
	walk = func(dir string) {
		dir = filepath.Clean(dir)
		if dir != home && !withinHome(home, dir) {
			return
		}
		if _, ok := seen[dir]; ok {
			return
		}
		seen[dir] = struct{}{}
		if dir != home {
			chmodAncestors(home, dir)
		}
		ents, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range ents {
			path := filepath.Join(dir, e.Name())
			if e.Type()&os.ModeSymlink != 0 {
				target, err := filepath.EvalSymlinks(path)
				if err != nil || !withinHome(home, target) {
					continue
				}
				st, err := os.Stat(target)
				if err != nil {
					continue
				}
				if st.IsDir() {
					walk(target)
				} else {
					_ = os.Chmod(target, 0644)
				}
				continue
			}
			if e.IsDir() {
				walk(path)
				continue
			}
			_ = os.Chmod(path, 0644)
		}
	}
	walk(root)
}

func chmodAncestors(home, dir string) {
	for p := filepath.Clean(dir); withinHome(home, p) && p != home; p = filepath.Dir(p) {
		_ = os.Chmod(p, 0755)
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
	}
}

func withinHome(home, path string) bool {
	home = filepath.Clean(home)
	path = filepath.Clean(path)
	if path == home {
		return true
	}
	rel, err := filepath.Rel(home, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

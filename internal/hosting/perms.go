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
	jail := home
	if root != home && !withinHome(home, root) {
		jail = root
	}
	seen := map[string]struct{}{}
	var walk func(string)
	walk = func(dir string) {
		dir = filepath.Clean(dir)
		if dir != jail && !withinHome(jail, dir) {
			return
		}
		if _, ok := seen[dir]; ok {
			return
		}
		seen[dir] = struct{}{}
		if dir != jail {
			chmodAncestors(jail, dir)
		}
		ents, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range ents {
			path := filepath.Join(dir, e.Name())
			if e.Type()&os.ModeSymlink != 0 {
				target, err := filepath.EvalSymlinks(path)
				if err != nil || !withinHome(jail, target) {
					continue
				}
				st, err := os.Stat(target)
				if err != nil {
					continue
				}
				if st.IsDir() {
					walk(target)
				} else {
					chmodWebFile(target)
				}
				continue
			}
			if e.IsDir() {
				walk(path)
				continue
			}
			chmodWebFile(path)
		}
	}
	walk(root)
}

// Keeps a file readable by the web server without clearing an execute bit.
// node_modules/.bin shims such as vite and vp are executed by npm run build.
func chmodWebFile(path string) {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return
	}
	_ = os.Chmod(path, 0644|(st.Mode().Perm()&0111))
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

// docInAccount is true when the document root lives under the account home.
// A path on another disk is not walked: recursive chown and chmod never finish on a large mount.
func docInAccount(home, doc string) bool {
	if home == "" || doc == "" {
		return false
	}
	return withinHome(home, doc)
}

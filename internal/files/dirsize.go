package files

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func followsDir(full string, info os.FileInfo) bool {
	if info != nil && info.IsDir() {
		return true
	}
	if info != nil && info.Mode()&os.ModeSymlink == 0 {
		return false
	}
	st, err := os.Stat(full)
	return err == nil && st.IsDir()
}

func dirBytes(root string) (int64, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return 0, err
	}
	walk := root
	if info.Mode()&os.ModeSymlink != 0 {
		st, err := os.Stat(root)
		if err != nil || !st.IsDir() {
			return info.Size(), nil
		}
		real, err := filepath.EvalSymlinks(root)
		if err != nil {
			return info.Size(), nil
		}
		walk = real
	} else if !info.IsDir() {
		return info.Size(), nil
	}
	if skipDirSize(walk) {
		return 0, nil
	}
	var n int64
	err = filepath.WalkDir(walk, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return fs.SkipDir
			}
			if info, err := d.Info(); err == nil {
				n += info.Size()
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		n += info.Size()
		return nil
	})
	return n, err
}

func skipDirSize(dir string) bool {
	p := path.Clean(filepath.ToSlash(dir))
	switch p {
	case "/proc", "/sys", "/dev", "/run":
		return true
	}
	return strings.HasPrefix(p, "/proc/") || strings.HasPrefix(p, "/sys/") || strings.HasPrefix(p, "/dev/") || strings.HasPrefix(p, "/run/")
}

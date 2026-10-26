package files

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func dirBytes(root string) (int64, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return 0, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return info.Size(), nil
	}
	var n int64
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
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

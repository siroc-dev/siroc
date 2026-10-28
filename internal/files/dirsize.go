package files

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
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

const dirSizeLimit = time.Minute

var errDirSizeLimit = errors.New("dir size limit")

func cleanDirSizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("invalid folder")
	}
	return name, nil
}

func dirBytes(root string) (int64, bool, error) {
	return dirBytesUntil(root, time.Now().Add(dirSizeLimit))
}

func dirBytesUntil(root string, deadline time.Time) (int64, bool, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return 0, false, err
	}
	walk := root
	if info.Mode()&os.ModeSymlink != 0 {
		st, err := os.Stat(root)
		if err != nil || !st.IsDir() {
			return info.Size(), false, nil
		}
		real, err := filepath.EvalSymlinks(root)
		if err != nil {
			return info.Size(), false, nil
		}
		walk = real
	} else if !info.IsDir() {
		return info.Size(), false, nil
	}
	if skipDirSize(walk) {
		return 0, false, nil
	}
	var n int64
	err = filepath.WalkDir(walk, func(path string, d fs.DirEntry, err error) error {
		if !deadline.IsZero() && time.Now().After(deadline) {
			return errDirSizeLimit
		}
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
	if errors.Is(err, errDirSizeLimit) {
		return n, true, nil
	}
	return n, false, err
}

func skipDirSize(dir string) bool {
	p := path.Clean(filepath.ToSlash(dir))
	switch p {
	case "/proc", "/sys", "/dev", "/run":
		return true
	}
	return strings.HasPrefix(p, "/proc/") || strings.HasPrefix(p, "/sys/") || strings.HasPrefix(p, "/dev/") || strings.HasPrefix(p, "/run/")
}

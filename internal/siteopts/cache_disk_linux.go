//go:build linux

package siteopts

import (
	"os"
	"path/filepath"
	"syscall"
)

func init() {
	lookupCacheDisk = statCacheDisk
}

func statCacheDisk(path string) uint64 {
	dir := filepath.Clean(path)
	for {
		if dir == "" || dir == "." {
			return 0
		}
		info, err := os.Stat(dir)
		if err == nil && info.IsDir() {
			var s syscall.Statfs_t
			if err := syscall.Statfs(dir, &s); err != nil || s.Blocks == 0 || s.Bsize <= 0 {
				return 0
			}
			return s.Blocks * uint64(s.Bsize)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return 0
		}
		dir = parent
	}
}

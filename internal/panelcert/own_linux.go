//go:build linux

package panelcert

import (
	"os"
	"syscall"
)

func matchDataOwner(dataDir string, paths ...string) {
	st, err := os.Stat(dataDir)
	if err != nil {
		return
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return
	}
	for _, path := range paths {
		_ = os.Chown(path, int(sys.Uid), int(sys.Gid))
	}
}

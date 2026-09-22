//go:build linux

package files

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

func (m *Manager) ownPath(abs string) {
	if abs == "" {
		return
	}
	root := m.HomeRoot
	if root == "" {
		root = "/home"
	}
	name, ok := accountFromHomePath(root, abs)
	if !ok {
		return
	}
	u, err := user.Lookup(name)
	if err != nil {
		return
	}
	uid, err1 := strconv.Atoi(u.Uid)
	gid, err2 := strconv.Atoi(u.Gid)
	if err1 != nil || err2 != nil || uid <= 0 {
		return
	}
	_ = exec.Command("chown", "-R", fmt.Sprintf("%d:%d", uid, gid), abs).Run()
	home := filepath.Join(root, name)
	if resolved, e := filepath.EvalSymlinks(home); e == nil {
		home = resolved
	}
	for p := filepath.Dir(abs); p != home && strings.HasPrefix(p, home); p = filepath.Dir(p) {
		_ = os.Chown(p, uid, gid)
		if p == "/" || filepath.Dir(p) == p {
			break
		}
	}
}

func permDenied(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "permission denied") || strings.Contains(s, "operation not permitted")
}

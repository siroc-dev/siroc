//go:build linux

package files

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func resolveRoot(rel string) (string, error) {
	if rel == "" {
		rel = "/"
	}
	if !strings.HasPrefix(rel, "/") {
		rel = "/" + rel
	}
	abs, err := filepath.Abs(filepath.Clean(rel))
	if err != nil {
		return "", err
	}
	if blockedRoot(abs) {
		return "", fmt.Errorf("path is not browsable")
	}
	return abs, nil
}

func blockedRoot(abs string) bool {
	switch abs {
	case "/proc", "/sys", "/dev":
		return true
	}
	for _, p := range []string{"/proc/", "/sys/", "/dev/"} {
		if strings.HasPrefix(abs, p) {
			return true
		}
	}
	return false
}

func protectedRoot(abs string) bool {
	switch filepath.Clean(abs) {
	case "/", "/boot", "/etc", "/home", "/usr", "/var", "/opt", "/root", "/bin", "/sbin", "/lib", "/lib64", "/dev", "/proc", "/sys", "/run", "/tmp":
		return true
	}
	return false
}

func (m *Manager) rootList(rel string) (*rpc.FileListResp, error) {
	abs, err := resolveRoot(rel)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("path is not a directory")
	}
	resp, err := runHelper(helperReq{Op: "list", Path: abs, Home: "/"})
	if err != nil {
		return nil, err
	}
	skip := map[string]bool{"proc": true, "sys": true, "dev": true}
	entries := resp.Entries[:0]
	for _, e := range resp.Entries {
		if abs == "/" && skip[e.Name] {
			continue
		}
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []rpc.FileEntry{}
	}
	sortEntries(entries)
	return &rpc.FileListResp{Path: filepath.ToSlash(abs), AbsPath: abs, Root: true, Entries: entries}, nil
}

func (m *Manager) rootRead(rel string) (*rpc.FileContentResp, error) {
	abs, err := resolveRoot(rel)
	if err != nil {
		return nil, err
	}
	resp, err := runHelper(helperReq{Op: "read", Path: abs, Home: "/"})
	if err != nil {
		return nil, err
	}
	return &rpc.FileContentResp{Path: filepath.ToSlash(abs), AbsPath: abs, Content: resp.Content}, nil
}

func (m *Manager) rootWrite(rel, content, encoding string) error {
	abs, err := resolveRoot(rel)
	if err != nil {
		return err
	}
	_, err = runHelper(helperReq{Op: "write", Path: abs, Home: "/", Content: content, Encoding: encoding})
	return err
}

func (m *Manager) rootMkdir(rel string) error {
	abs, err := resolveRoot(rel)
	if err != nil {
		return err
	}
	_, err = runHelper(helperReq{Op: "mkdir", Path: abs, Home: "/"})
	return err
}

func (m *Manager) rootDelete(rel string) error {
	abs, err := resolveRoot(rel)
	if err != nil {
		return err
	}
	if protectedRoot(abs) {
		return fmt.Errorf("cannot delete system path %s", abs)
	}
	_, err = runHelper(helperReq{Op: "delete", Path: abs, Home: "/"})
	return err
}

func (m *Manager) rootRename(rel, dest string) error {
	src, err := resolveRoot(rel)
	if err != nil {
		return err
	}
	dst, err := resolveRoot(dest)
	if err != nil {
		return err
	}
	if protectedRoot(src) || protectedRoot(dst) {
		return fmt.Errorf("cannot rename a system path")
	}
	_, err = runHelper(helperReq{Op: "rename", Path: src, Dest: dst, Home: "/"})
	return err
}

func (m *Manager) rootChmod(rel, mode string) error {
	abs, err := resolveRoot(rel)
	if err != nil {
		return err
	}
	if _, err := strconv.ParseUint(mode, 8, 32); err != nil {
		return fmt.Errorf("invalid mode")
	}
	_, err = runHelper(helperReq{Op: "chmod", Path: abs, Home: "/", Mode: mode})
	return err
}

func (m *Manager) ResolveLSP(username, workspace string, root bool) (dir string, uid, gid int, home string, err error) {
	if root {
		abs, err := resolveRoot(workspace)
		if err != nil {
			return "", 0, 0, "", err
		}
		st, err := os.Stat(abs)
		if err != nil {
			return "", 0, 0, "", err
		}
		if !st.IsDir() {
			abs = filepath.Dir(abs)
		}
		return abs, 0, 0, "/", nil
	}
	homeAbs, home, uid, gid, err := m.resolve(username, ".")
	if err != nil {
		return "", 0, 0, "", err
	}
	if workspace == "" {
		return homeAbs, uid, gid, home, nil
	}
	ws, err := filepath.Abs(filepath.Clean(workspace))
	if err != nil {
		return "", 0, 0, "", err
	}
	rel, err := filepath.Rel(home, ws)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", 0, 0, "", fmt.Errorf("workspace is outside the account home")
	}
	st, err := os.Stat(ws)
	if err != nil {
		return "", 0, 0, "", err
	}
	if !st.IsDir() {
		ws = filepath.Dir(ws)
	}
	return ws, uid, gid, home, nil
}

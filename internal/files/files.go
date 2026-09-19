//go:build linux

package files

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/validate"
)

type Manager struct {
	HomeRoot string
}

type helperReq struct {
	Op       string `json:"op"`
	Path     string `json:"path"`
	Dest     string `json:"dest,omitempty"`
	Content  string `json:"content,omitempty"`
	Encoding string `json:"encoding,omitempty"`
	Mode     string `json:"mode,omitempty"`
	URL      string `json:"url,omitempty"`
	Query    string `json:"query,omitempty"`
	Home     string `json:"home"`
}

type helperResp struct {
	OK        bool               `json:"ok"`
	Error     string             `json:"error,omitempty"`
	Entries   []rpc.FileEntry    `json:"entries,omitempty"`
	Content   string             `json:"content,omitempty"`
	Dest      string             `json:"dest,omitempty"`
	Hits      []rpc.FileSearchHit `json:"hits,omitempty"`
	Truncated bool               `json:"truncated,omitempty"`
}

func (m *Manager) resolve(username, rel string) (abs, home string, uid, gid int, err error) {
	if err := validate.LinuxUser(username); err != nil {
		return "", "", 0, 0, err
	}
	u, err := user.Lookup(username)
	if err != nil {
		return "", "", 0, 0, err
	}
	uid, _ = strconv.Atoi(u.Uid)
	gid, _ = strconv.Atoi(u.Gid)
	home, err = filepath.Abs(u.HomeDir)
	if err != nil {
		return "", "", 0, 0, err
	}
	if resolved, e := filepath.EvalSymlinks(home); e == nil {
		home = resolved
	}
	if rel == "" || rel == "/" {
		rel = "."
	}
	rel = strings.TrimPrefix(rel, "/")
	candidate := filepath.Join(home, rel)
	abs, err = filepath.Abs(candidate)
	if err != nil {
		return "", "", 0, 0, err
	}
	check := abs
	if st, e := os.Lstat(abs); e == nil {
		if st.Mode()&os.ModeSymlink != 0 {
			if r, e2 := filepath.EvalSymlinks(abs); e2 == nil {
				check = r
			}
		} else if r, e2 := filepath.EvalSymlinks(abs); e2 == nil {
			check = r
		}
	} else if os.IsNotExist(e) {
		if r, e2 := filepath.EvalSymlinks(filepath.Dir(abs)); e2 == nil {
			check = filepath.Join(r, filepath.Base(abs))
		}
	}
	relToHome, err := filepath.Rel(home, check)
	if err != nil || strings.HasPrefix(relToHome, "..") {
		return "", "", 0, 0, fmt.Errorf("path is outside the account home")
	}
	return abs, home, uid, gid, nil
}

func (m *Manager) call(uid, gid int, req helperReq) (*helperResp, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(self, "file-helper")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)},
	}
	cmd.Stdin = bytes.NewReader(raw)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("file helper: %s", msg)
	}
	var resp helperResp
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("%s", resp.Error)
	}
	return &resp, nil
}

func (m *Manager) List(username, rel string, root bool) (*rpc.FileListResp, error) {
	if root {
		return m.rootList(rel)
	}
	abs, home, uid, gid, err := m.resolve(username, rel)
	if err != nil {
		return nil, err
	}
	resp, err := m.call(uid, gid, helperReq{Op: "list", Path: abs, Home: home})
	if err != nil {
		return nil, err
	}
	show, _ := filepath.Rel(home, abs)
	showPath := "/"
	if show != "." {
		showPath = "/" + filepath.ToSlash(show)
	}
	if resp.Entries == nil {
		resp.Entries = []rpc.FileEntry{}
	}
	return &rpc.FileListResp{Path: showPath, AbsPath: abs, Entries: resp.Entries}, nil
}

func (m *Manager) Read(username, rel string, root bool) (*rpc.FileContentResp, error) {
	if root {
		return m.rootRead(rel)
	}
	abs, home, uid, gid, err := m.resolve(username, rel)
	if err != nil {
		return nil, err
	}
	resp, err := m.call(uid, gid, helperReq{Op: "read", Path: abs, Home: home})
	if err != nil {
		return nil, err
	}
	return &rpc.FileContentResp{Path: rel, AbsPath: abs, Content: resp.Content}, nil
}

func (m *Manager) Write(username, rel, content string, root bool) error {
	if root {
		return m.rootWrite(rel, content, "")
	}
	abs, home, uid, gid, err := m.resolve(username, rel)
	if err != nil {
		return err
	}
	_, err = m.call(uid, gid, helperReq{Op: "write", Path: abs, Home: home, Content: content})
	return err
}

func (m *Manager) Mkdir(username, rel string, root bool) error {
	if root {
		return m.rootMkdir(rel)
	}
	abs, home, uid, gid, err := m.resolve(username, rel)
	if err != nil {
		return err
	}
	_, err = m.call(uid, gid, helperReq{Op: "mkdir", Path: abs, Home: home})
	return err
}

func (m *Manager) Delete(username, rel string, root bool) error {
	if root {
		return m.rootDelete(rel)
	}
	abs, home, uid, gid, err := m.resolve(username, rel)
	if err != nil {
		return err
	}
	if abs == home {
		return fmt.Errorf("cannot delete home directory")
	}
	_, err = m.call(uid, gid, helperReq{Op: "delete", Path: abs, Home: home})
	return err
}

func (m *Manager) Rename(username, rel, dest string, root bool) error {
	if root {
		return m.rootRename(rel, dest)
	}
	src, home, uid, gid, err := m.resolve(username, rel)
	if err != nil {
		return err
	}
	dst, _, _, _, err := m.resolve(username, dest)
	if err != nil {
		return err
	}
	_, err = m.call(uid, gid, helperReq{Op: "rename", Path: src, Dest: dst, Home: home})
	return err
}

func (m *Manager) Chmod(username, rel, mode string, root bool) error {
	if root {
		return m.rootChmod(rel, mode)
	}
	abs, home, uid, gid, err := m.resolve(username, rel)
	if err != nil {
		return err
	}
	if _, err := strconv.ParseUint(mode, 8, 32); err != nil {
		return fmt.Errorf("invalid mode")
	}
	_, err = m.call(uid, gid, helperReq{Op: "chmod", Path: abs, Home: home, Mode: mode})
	return err
}

func (m *Manager) Upload(username, rel string, r io.Reader, root bool) error {
	const max = 512 << 20
	var abs string
	uid, gid := 0, 0
	if root {
		p, err := resolveRoot(rel)
		if err != nil {
			return err
		}
		if protectedRoot(p) {
			return fmt.Errorf("cannot write to a system path")
		}
		abs = p
	} else {
		p, _, u, g, err := m.resolve(username, rel)
		if err != nil {
			return err
		}
		abs, uid, gid = p, u, g
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0750); err != nil {
		return err
	}
	tmp := abs + ".cp-uploading"
	_ = os.Remove(tmp)
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(r, max+1))
	closeErr := f.Close()
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if n > max {
		_ = os.Remove(tmp)
		return fmt.Errorf("file too large to upload (max 512MB)")
	}
	_ = os.Remove(abs)
	if err := os.Rename(tmp, abs); err != nil {
		if cpErr := exec.Command("cp", "-a", tmp, abs).Run(); cpErr != nil {
			_ = os.Remove(tmp)
			return err
		}
		_ = os.Remove(tmp)
	}
	if uid > 0 {
		_ = os.Chown(abs, uid, gid)
	}
	return nil
}

func HelperMain() {
	var req helperReq
	if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
		writeHelper(helperResp{Error: err.Error()})
		return
	}
	if err := stillJailed(req.Home, req.Path); err != nil {
		writeHelper(helperResp{Error: err.Error()})
		return
	}
	if req.Dest != "" {
		if err := stillJailed(req.Home, req.Dest); err != nil {
			writeHelper(helperResp{Error: err.Error()})
			return
		}
	}
	resp, err := runHelper(req)
	if err != nil {
		writeHelper(helperResp{Error: err.Error()})
		return
	}
	writeHelper(resp)
}

func stillJailed(home, path string) error {
	rel, err := filepath.Rel(home, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("path is outside the account home")
	}
	return nil
}

func runHelper(req helperReq) (helperResp, error) {
	switch req.Op {
	case "list":
		fis, err := os.ReadDir(req.Path)
		if err != nil {
			return helperResp{}, err
		}
		var entries []rpc.FileEntry
		for _, e := range fis {
			info, err := e.Info()
			if err != nil {
				continue
			}
			rel, _ := filepath.Rel(req.Home, filepath.Join(req.Path, info.Name()))
			entries = append(entries, rpc.FileEntry{
				Name:    info.Name(),
				Path:    "/" + filepath.ToSlash(rel),
				IsDir:   info.IsDir(),
				Size:    info.Size(),
				Mode:    fmt.Sprintf("%04o", info.Mode().Perm()),
				ModTime: info.ModTime().UTC().Format(time.RFC3339),
			})
		}
		if entries == nil {
			entries = []rpc.FileEntry{}
		}
		sortEntries(entries)
		return helperResp{OK: true, Entries: entries}, nil
	case "read":
		st, err := os.Stat(req.Path)
		if err != nil {
			return helperResp{}, err
		}
		if st.IsDir() {
			return helperResp{}, fmt.Errorf("path is a directory")
		}
		if st.Size() > 2<<20 {
			return helperResp{}, fmt.Errorf("file too large to edit (max 2MB)")
		}
		b, err := os.ReadFile(req.Path)
		if err != nil {
			return helperResp{}, err
		}
		if !validate.Printable(string(b)) {
			return helperResp{}, fmt.Errorf("file is binary")
		}
		return helperResp{OK: true, Content: string(b)}, nil
	case "write":
		payload := []byte(req.Content)
		if req.Encoding == "base64" {
			decoded, err := base64.StdEncoding.DecodeString(req.Content)
			if err != nil {
				return helperResp{}, fmt.Errorf("invalid upload encoding")
			}
			payload = decoded
		}
		if err := os.MkdirAll(filepath.Dir(req.Path), 0750); err != nil {
			return helperResp{}, err
		}
		if err := os.WriteFile(req.Path, payload, 0640); err != nil {
			return helperResp{}, err
		}
		return helperResp{OK: true}, nil
	case "mkdir":
		if err := os.MkdirAll(req.Path, 0750); err != nil {
			return helperResp{}, err
		}
		return helperResp{OK: true}, nil
	case "delete":
		if err := os.RemoveAll(req.Path); err != nil {
			return helperResp{}, err
		}
		return helperResp{OK: true}, nil
	case "rename":
		if err := os.MkdirAll(filepath.Dir(req.Dest), 0750); err != nil {
			return helperResp{}, err
		}
		dest := uniquePath(req.Dest)
		if err := os.Rename(req.Path, dest); err != nil {
			if err2 := copyPath(req.Path, dest); err2 != nil {
				return helperResp{}, err
			}
			if err := os.RemoveAll(req.Path); err != nil {
				return helperResp{}, err
			}
		}
		return helperResp{OK: true, Dest: dest}, nil
	case "chmod":
		parsed, err := strconv.ParseUint(req.Mode, 8, 32)
		if err != nil {
			return helperResp{}, fmt.Errorf("invalid mode")
		}
		if err := os.Chmod(req.Path, fs.FileMode(parsed)); err != nil {
			return helperResp{}, err
		}
		return helperResp{OK: true}, nil
	case "extract":
		dest := req.Dest
		if dest == "" {
			dest = uniquePath(filepath.Join(filepath.Dir(req.Path), stripArchiveName(filepath.Base(req.Path))))
		}
		if err := extractArchive(req.Path, dest); err != nil {
			return helperResp{}, err
		}
		return helperResp{OK: true, Dest: dest}, nil
	case "copy":
		if err := copyPath(req.Path, req.Dest); err != nil {
			return helperResp{}, err
		}
		return helperResp{OK: true, Dest: req.Dest}, nil
	case "fetch":
		if err := curlFetch(req.Path, req.URL); err != nil {
			return helperResp{}, err
		}
		return helperResp{OK: true, Dest: req.Path}, nil
	case "search":
		hits, trunc := runSearch(req.Path, req.Home, req.Query)
		return helperResp{OK: true, Hits: hits, Truncated: trunc}, nil
	default:
		return helperResp{}, fmt.Errorf("unknown file op")
	}
}

func sortEntries(entries []rpc.FileEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
}

func writeHelper(resp helperResp) {
	if resp.Error == "" {
		resp.OK = true
	}
	_ = json.NewEncoder(os.Stdout).Encode(resp)
}

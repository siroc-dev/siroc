//go:build linux

package rclone

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func Status() (*rpc.RcloneStatus, error) {
	p, err := exec.LookPath("rclone")
	if err != nil {
		return &rpc.RcloneStatus{Message: "rclone is not installed. Install it from Software."}, nil
	}
	verOut, _ := exec.Command(p, "version").CombinedOutput()
	ver := ""
	if lines := strings.Split(strings.TrimSpace(string(verOut)), "\n"); len(lines) > 0 {
		ver = strings.TrimSpace(lines[0])
	}
	list, err := remotes()
	st := &rpc.RcloneStatus{Installed: true, Version: ver, Remotes: list}
	if err != nil {
		st.Message = err.Error()
	}
	return st, nil
}

func remotes() ([]rpc.RcloneRemote, error) {
	dump, err := loadDump()
	if err != nil {
		return nil, err
	}
	list := make([]rpc.RcloneRemote, 0, len(dump))
	for name, cfg := range dump {
		params, hasSecret, hasKey := RedactRemote(cfg["type"], cfg)
		list = append(list, rpc.RcloneRemote{
			Name:      name,
			Type:      cfg["type"],
			Params:    params,
			HasSecret: hasSecret,
			HasKey:    hasKey,
		})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list, nil
}

func loadDump() (map[string]map[string]string, error) {
	out, err := exec.Command("rclone", "config", "dump").CombinedOutput()
	text := strings.TrimSpace(string(out))
	if text == "" || text == "{}" || text == "null" {
		if err != nil {
			return nil, fmt.Errorf("rclone config: %s", err.Error())
		}
		return map[string]map[string]string{}, nil
	}
	var raw map[string]map[string]any
	if jerr := json.Unmarshal([]byte(text), &raw); jerr != nil {
		return nil, fmt.Errorf("rclone config: %s", clip(text))
	}
	dump := make(map[string]map[string]string, len(raw))
	for name, cfg := range raw {
		flat := make(map[string]string, len(cfg))
		for key, value := range cfg {
			flat[key] = scalarString(value)
		}
		dump[name] = flat
	}
	return dump, nil
}

func scalarString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func Create(req rpc.RcloneRemoteReq) error {
	if _, err := exec.LookPath("rclone"); err != nil {
		return fmt.Errorf("rclone is not installed. Install it from Software.")
	}
	if err := validRemoteName(req.Name); err != nil {
		return err
	}
	if req.Params == nil {
		req.Params = map[string]string{}
	}
	delete(req.Params, "key_file")
	var pemBytes []byte
	if strings.TrimSpace(req.Key) != "" {
		if req.Type != "sftp" {
			return fmt.Errorf("a private key is only used with SFTP")
		}
		var err error
		pemBytes, err = NormalizePrivateKey([]byte(req.Key), req.KeyPass)
		if err != nil {
			return err
		}
		req.Params["key_file"] = keyPath(req.Name)
	}
	args, err := CreateArgs(req.Name, req.Type, req.Params)
	if err != nil {
		return err
	}
	if pemBytes != nil {
		if err := saveKey(req.Name, pemBytes); err != nil {
			return err
		}
	}
	out, err := exec.Command("rclone", args...).CombinedOutput()
	if err != nil {
		if pemBytes != nil {
			_ = os.Remove(keyPath(req.Name))
		}
		msg := clip(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("rclone: %s", msg)
	}
	if pemBytes == nil {
		_ = os.Remove(keyPath(req.Name))
	}
	return nil
}

func Update(req rpc.RcloneRemoteReq) error {
	if _, err := exec.LookPath("rclone"); err != nil {
		return fmt.Errorf("rclone is not installed. Install it from Software.")
	}
	if err := validRemoteName(req.Name); err != nil {
		return err
	}
	dump, err := loadDump()
	if err != nil {
		return err
	}
	cfg, ok := dump[req.Name]
	if !ok {
		return fmt.Errorf("remote not found")
	}
	if req.Type == "" {
		req.Type = cfg["type"]
	}
	if cfg["type"] != req.Type {
		return fmt.Errorf("remote type cannot be changed")
	}
	if req.Params == nil {
		req.Params = map[string]string{}
	}
	delete(req.Params, "key_file")
	var pemBytes []byte
	if strings.TrimSpace(req.Key) != "" {
		if req.Type != "sftp" {
			return fmt.Errorf("a private key is only used with SFTP")
		}
		pemBytes, err = NormalizePrivateKey([]byte(req.Key), req.KeyPass)
		if err != nil {
			return err
		}
		req.Params["key_file"] = keyPath(req.Name)
	} else if req.ClearKey && req.Type == "sftp" {
		req.Params["key_file"] = ""
	}
	args, err := UpdateArgs(req.Name, req.Type, req.Params)
	if err != nil {
		return err
	}
	var restore func()
	if pemBytes != nil {
		restore, err = replaceKey(req.Name, pemBytes)
		if err != nil {
			return err
		}
	}
	if args != nil {
		out, runErr := exec.Command("rclone", args...).CombinedOutput()
		if runErr != nil {
			if restore != nil {
				restore()
			}
			msg := clip(string(out))
			if msg == "" {
				msg = runErr.Error()
			}
			return fmt.Errorf("rclone: %s", msg)
		}
	}
	if req.ClearKey && pemBytes == nil {
		_ = os.Remove(keyPath(req.Name))
	}
	return nil
}

func Delete(name string) error {
	if _, err := exec.LookPath("rclone"); err != nil {
		return fmt.Errorf("rclone is not installed. Install it from Software.")
	}
	if err := validRemoteName(name); err != nil {
		return err
	}
	out, err := exec.Command("rclone", "config", "delete", name).CombinedOutput()
	if err != nil {
		msg := clip(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("rclone: %s", msg)
	}
	_ = os.Remove(keyPath(name))
	return nil
}

func keyPath(name string) string {
	return filepath.Join("/var/lib/siroc/rclone/keys", name+".pem")
}

func saveKey(name string, pemBytes []byte) error {
	dir := filepath.Dir(keyPath(name))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.WriteFile(keyPath(name), pemBytes, 0600)
}

func replaceKey(name string, pemBytes []byte) (func(), error) {
	path := keyPath(name)
	old, err := os.ReadFile(path)
	had := err == nil
	if err := saveKey(name, pemBytes); err != nil {
		return nil, err
	}
	return func() {
		if had {
			_ = os.WriteFile(path, old, 0600)
			return
		}
		_ = os.Remove(path)
	}, nil
}

var (
	jobMu   sync.Mutex
	current *runState
)

type runState struct {
	mu   sync.Mutex
	snap rpc.RcloneRunResp
	buf  strings.Builder
}

func List(req rpc.RcloneListReq) (*rpc.RcloneListResp, error) {
	if _, err := exec.LookPath("rclone"); err != nil {
		return nil, fmt.Errorf("rclone is not installed. Install it from Software.")
	}
	target, err := ListTarget(req.Remote, req.Path)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "rclone", "lsjson", target, "--dirs-only", "--max-depth", "1", "--no-modtime", "--retries", "1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("listing folders timed out")
		}
		msg := clip(stderr.String())
		if msg == "" {
			msg = clip(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("rclone: %s", msg)
	}
	dirs, err := ParseDirs(stdout.Bytes())
	if err != nil {
		return nil, err
	}
	return &rpc.RcloneListResp{Path: target, Dirs: dirs}, nil
}

func Start(req rpc.RcloneRunReq) (*rpc.RcloneRunResp, error) {
	if _, err := exec.LookPath("rclone"); err != nil {
		return nil, fmt.Errorf("rclone is not installed. Install it from Software.")
	}
	args, slow, err := RunArgs(req.Action, req.Source, req.Dest)
	if err != nil {
		return nil, err
	}
	args = append(args, ProgressFlags(req.Action)...)
	jobMu.Lock()
	if current != nil && current.running() {
		jobMu.Unlock()
		return nil, fmt.Errorf("a transfer is already running")
	}
	st := &runState{snap: rpc.RcloneRunResp{
		Running: true,
		Action:  req.Action,
		Source:  strings.TrimSpace(req.Source),
		Dest:    strings.TrimSpace(req.Dest),
	}}
	current = st
	jobMu.Unlock()
	go st.execute(args, slow)
	out := st.snapshot()
	return &out, nil
}

func Job() rpc.RcloneRunResp {
	jobMu.Lock()
	st := current
	jobMu.Unlock()
	if st == nil {
		return rpc.RcloneRunResp{}
	}
	return st.snapshot()
}

func (st *runState) running() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.snap.Running
}

func (st *runState) snapshot() rpc.RcloneRunResp {
	st.mu.Lock()
	defer st.mu.Unlock()
	out := st.snap
	if len(st.snap.Files) > 0 {
		out.Files = append([]rpc.RcloneFileProgress(nil), st.snap.Files...)
	}
	return out
}

func (st *runState) execute(args []string, slow bool) {
	limit := 2 * time.Minute
	if slow {
		limit = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, "rclone", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		st.finish(false, err.Error())
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		st.finish(false, err.Error())
		return
	}
	if err := cmd.Start(); err != nil {
		st.finish(false, err.Error())
		return
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		st.read(stdout)
	}()
	go func() {
		defer wg.Done()
		st.read(stderr)
	}()
	wg.Wait()
	err = cmd.Wait()
	if err == nil {
		st.finish(true, "")
		return
	}
	msg := "rclone failed"
	if ctx.Err() == context.DeadlineExceeded {
		msg = "rclone timed out"
	}
	st.finish(false, msg)
}

func (st *runState) read(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		st.note(sc.Text())
	}
}

func (st *runState) note(line string) {
	logLine, prog := Interpret(line)
	st.mu.Lock()
	defer st.mu.Unlock()
	if prog != nil {
		st.snap.Bytes = prog.Bytes
		st.snap.TotalBytes = prog.TotalBytes
		st.snap.Speed = prog.Speed
		st.snap.Transfers = prog.Transfers
		st.snap.TotalTransfers = prog.TotalTransfers
		st.snap.Listed = prog.Listed
		st.snap.Percent = prog.Percent
		st.snap.Files = prog.Files
		return
	}
	if logLine == "" {
		return
	}
	if st.buf.Len() > 0 {
		st.buf.WriteByte('\n')
	}
	st.buf.WriteString(logLine)
	if st.buf.Len() > 48*1024 {
		text := st.buf.String()
		st.buf.Reset()
		st.buf.WriteString(text[len(text)-48*1024:])
	}
	st.snap.Output = st.buf.String()
}

func (st *runState) finish(ok bool, message string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.snap.Running = false
	st.snap.OK = ok
	st.snap.Message = message
	if ok && (st.snap.Percent > 0 || st.snap.TotalBytes > 0) {
		st.snap.Percent = 100
		st.snap.Files = nil
	}
}

func clip(s string) string {
	s = strings.TrimSpace(s)
	const max = 48 * 1024
	if len(s) <= max {
		return s
	}
	return s[len(s)-max:]
}

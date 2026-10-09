//go:build linux

package rclone

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sort"
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
	out, err := exec.Command("rclone", "config", "dump").CombinedOutput()
	text := strings.TrimSpace(string(out))
	if text == "" {
		if err != nil {
			return nil, fmt.Errorf("rclone config: %s", err.Error())
		}
		return nil, nil
	}
	var dump map[string]struct {
		Type string `json:"type"`
	}
	if jerr := json.Unmarshal([]byte(text), &dump); jerr != nil {
		if err != nil {
			return nil, fmt.Errorf("rclone config: %s", clip(text))
		}
		return nil, fmt.Errorf("rclone config: %s", clip(text))
	}
	list := make([]rpc.RcloneRemote, 0, len(dump))
	for name, cfg := range dump {
		list = append(list, rpc.RcloneRemote{Name: name, Type: cfg.Type})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list, nil
}

func Create(req rpc.RcloneRemoteReq) error {
	if _, err := exec.LookPath("rclone"); err != nil {
		return fmt.Errorf("rclone is not installed. Install it from Software.")
	}
	args, err := CreateArgs(req.Name, req.Type, req.Params)
	if err != nil {
		return err
	}
	out, err := exec.Command("rclone", args...).CombinedOutput()
	if err != nil {
		msg := clip(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("rclone: %s", msg)
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
	return nil
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

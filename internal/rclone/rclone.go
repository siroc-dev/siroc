//go:build linux

package rclone

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
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

func Run(req rpc.RcloneRunReq) (*rpc.RcloneRunResp, error) {
	if _, err := exec.LookPath("rclone"); err != nil {
		return nil, fmt.Errorf("rclone is not installed. Install it from Software.")
	}
	args, slow, err := RunArgs(req.Action, req.Source, req.Dest)
	if err != nil {
		return nil, err
	}
	limit := 2 * time.Minute
	if slow {
		limit = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	out, err := exec.CommandContext(ctx, "rclone", args...).CombinedOutput()
	resp := &rpc.RcloneRunResp{Output: clip(string(out))}
	if err != nil {
		msg := resp.Output
		if msg == "" {
			msg = err.Error()
		}
		resp.Message = msg
		return resp, fmt.Errorf("rclone: %s", msg)
	}
	resp.OK = true
	return resp, nil
}

func clip(s string) string {
	s = strings.TrimSpace(s)
	const max = 48 * 1024
	if len(s) <= max {
		return s
	}
	return s[len(s)-max:]
}

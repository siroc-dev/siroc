//go:build linux

package weblog

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func SystemLogs(id, group string, max int) *rpc.SystemLogResp {
	files := listSystemLogs()
	out := &rpc.SystemLogResp{Files: files}
	id = strings.TrimSpace(id)
	group = strings.TrimSpace(group)
	if id == "" && group != "" {
		for _, f := range files {
			if f.Group == group {
				id = f.ID
				break
			}
		}
	}
	if id == "" {
		if len(files) > 0 {
			id = files[0].ID
		} else {
			return out
		}
	}
	var current *rpc.SystemLogFile
	for i := range files {
		if files[i].ID == id {
			current = &files[i]
			break
		}
	}
	if current == nil {
		out.Message = "Unknown log"
		return out
	}
	out.Current = current
	if !current.Exists && current.Kind == "file" {
		out.Message = "Log file is not present yet."
		return out
	}
	content, size, trunc, err := readSystemLog(*current, max)
	if err != nil {
		out.Message = err.Error()
		return out
	}
	if size > 0 {
		current.Size = size
	}
	out.Content = content
	out.Truncated = trunc
	return out
}

func listSystemLogs() []rpc.SystemLogFile {
	var out []rpc.SystemLogFile
	seen := map[string]bool{}
	for _, s := range systemLogCatalog() {
		row := describeLog(s)
		if seen[row.ID] {
			continue
		}
		seen[row.ID] = true
		out = append(out, row)
	}
	matches, _ := filepath.Glob("/var/log/php*-fpm.log")
	for _, p := range matches {
		id, ok := phpFpmLogID(filepath.Base(p))
		if !ok || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, describeLog(systemLogSpec{
			ID:    id,
			Group: "server",
			Title: "PHP-FPM " + strings.TrimPrefix(id, "php-fpm-"),
			Path:  p,
			Kind:  "file",
		}))
	}
	return out
}

func describeLog(s systemLogSpec) rpc.SystemLogFile {
	row := rpc.SystemLogFile{ID: s.ID, Group: s.Group, Title: s.Title, Path: s.Path, Kind: s.Kind}
	switch s.Kind {
	case "journal":
		row.Exists = journalUnitKnown(s.Unit)
		row.Path = "journal:" + s.Unit
	case "command":
		row.Exists = true
		row.Path = "command:last"
	default:
		if st, err := os.Stat(s.Path); err == nil && !st.IsDir() {
			row.Exists = true
			row.Size = st.Size()
			row.ModTime = st.ModTime().UTC().Format(time.RFC3339)
		}
	}
	return row
}

func readSystemLog(file rpc.SystemLogFile, max int) (string, int64, bool, error) {
	spec, ok := lookupSystemLog(file.ID)
	if !ok {
		if strings.HasPrefix(file.ID, "php-fpm-") && file.Kind == "file" && strings.HasPrefix(file.Path, "/var/log/php") && strings.HasSuffix(file.Path, "-fpm.log") {
			return ReadTail(file.Path, max)
		}
		return "", 0, false, os.ErrNotExist
	}
	switch spec.Kind {
	case "journal":
		text, err := readJournal(spec.Unit, 400)
		return text, int64(len(text)), false, err
	case "command":
		text, err := readLast(80)
		return text, int64(len(text)), false, err
	default:
		return ReadTail(spec.Path, max)
	}
}

func readJournal(unit string, lines int) (string, error) {
	if unit == "" || strings.ContainsAny(unit, " \t/\\") {
		return "", os.ErrInvalid
	}
	if lines < 50 {
		lines = 50
	}
	if lines > 800 {
		lines = 800
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "journalctl", "-u", unit, "-n", strconv.Itoa(lines), "--no-pager", "-o", "short-iso")
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil && text == "" {
		return "", err
	}
	return text, nil
}

func readLast(n int) (string, error) {
	if n < 10 {
		n = 10
	}
	if n > 200 {
		n = 200
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "last", "-n", strconv.Itoa(n), "-w")
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil && text == "" {
		return "", err
	}
	return text, nil
}

func journalUnitKnown(unit string) bool {
	if unit == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "systemctl", "show", "-p", "LoadState", "--value", unit+".service")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return bytes.Contains(out, []byte("loaded"))
	}
	return strings.TrimSpace(string(out)) == "loaded"
}

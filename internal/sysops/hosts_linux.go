//go:build linux

package sysops

import (
	"os"
	"path/filepath"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func ListHostEntries() (*rpc.HostsResp, error) {
	text, err := readHostsFile(hostsPath)
	if err != nil {
		return nil, err
	}
	return &rpc.HostsResp{Entries: ListHostsText(text)}, nil
}

func ChangeHosts(op string, id int, ip string, names []string) (*rpc.HostsResp, error) {
	text, err := readHostsFile(hostsPath)
	if err != nil {
		return nil, err
	}
	next, entries, err := MutateHosts(text, op, id, ip, names, time.Now())
	if err != nil {
		return nil, err
	}
	if err := writeHostsFile(hostsPath, next); err != nil {
		return nil, err
	}
	return &rpc.HostsResp{Entries: entries}, nil
}

const hostsPath = "/etc/hosts"

func readHostsFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(b), nil
}

func writeHostsFile(path, text string) error {
	if text != "" && text[len(text)-1] != '\n' {
		text += "\n"
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".hosts-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := f.WriteString(text); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Chmod(0644); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	return nil
}

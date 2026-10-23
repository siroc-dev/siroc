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
	if err := saveManagedHosts(next); err != nil {
		return nil, err
	}
	return &rpc.HostsResp{Entries: entries}, nil
}

const managedHostsPath = "/var/lib/siroc/hosts-managed"

const cloudInitHostsCfg = "# Added by siroc. cloud-init rewrites /etc/hosts on boot and drops addresses saved in System tools.\nmanage_etc_hosts: false\n"

// RestoreManagedHosts puts panel rows back after a boot script replaces /etc/hosts,
// and tells cloud-init to stop doing that on the next boot.
func RestoreManagedHosts() error {
	stopCloudInitHosts()
	live, err := readHostsFile(hostsPath)
	if err != nil {
		return err
	}
	stored, err := readHostsFile(managedHostsPath)
	if err != nil {
		return err
	}
	snap := unionManaged(stored, live)
	if snap != stored {
		if err := saveManagedText(snap); err != nil {
			return err
		}
	}
	merged, changed := mergeManagedHosts(live, snap)
	if !changed {
		return nil
	}
	return writeHostsFile(hostsPath, merged)
}

func saveManagedHosts(hostsText string) error {
	return saveManagedText(managedHostText(hostsText))
}

func saveManagedText(text string) error {
	if err := os.MkdirAll(filepath.Dir(managedHostsPath), 0755); err != nil {
		return err
	}
	return writeHostsFile(managedHostsPath, text)
}

func stopCloudInitHosts() {
	if _, err := os.Stat("/etc/cloud"); err != nil {
		if _, err := os.Stat("/usr/bin/cloud-init"); err != nil {
			return
		}
	}
	dir := "/etc/cloud/cloud.cfg.d"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	path := filepath.Join(dir, "99-siroc-hosts.cfg")
	if b, err := os.ReadFile(path); err == nil && string(b) == cloudInitHostsCfg {
		return
	}
	_ = os.WriteFile(path, []byte(cloudInitHostsCfg), 0644)
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

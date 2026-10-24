//go:build linux

package sysops

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const netplanDir = "/etc/netplan"

const cloudInitNetCfg = "# Added by siroc. cloud-init rewrites netplan on boot and drops addresses saved in System tools.\nnetwork:\n  config: disabled\n"

func useNetplan() bool {
	if _, err := exec.LookPath("netplan"); err != nil {
		return false
	}
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return false
	}
	return osReleaseID(string(b)) == "ubuntu"
}

func readNetplanFiles() (map[string]string, error) {
	entries, err := os.ReadDir(netplanDir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	out := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(netplanDir, name))
		if err != nil {
			return nil, err
		}
		out[name] = string(b)
	}
	return out, nil
}

func commitNetplan(plan netplanPlan) error {
	if len(plan.Update) == 0 && len(plan.Delete) == 0 {
		return nil
	}
	if err := os.MkdirAll(netplanDir, 0755); err != nil {
		return err
	}
	type snap struct {
		name string
		data []byte
		ok   bool
		mode os.FileMode
	}
	var snaps []snap
	restore := func() {
		for i := len(snaps) - 1; i >= 0; i-- {
			s := snaps[i]
			path := filepath.Join(netplanDir, s.name)
			if !s.ok {
				_ = os.Remove(path)
				continue
			}
			_ = os.WriteFile(path, s.data, s.mode)
		}
	}
	remember := func(name string) snap {
		path := filepath.Join(netplanDir, name)
		s := snap{name: name, mode: 0600}
		if b, err := os.ReadFile(path); err == nil {
			s.data = b
			s.ok = true
			if st, err := os.Stat(path); err == nil {
				s.mode = st.Mode().Perm()
			}
		}
		snaps = append(snaps, s)
		return s
	}
	for name, content := range plan.Update {
		remember(name)
		mode := os.FileMode(0600)
		if st, err := os.Stat(filepath.Join(netplanDir, name)); err == nil {
			mode = st.Mode().Perm()
		}
		if err := os.WriteFile(filepath.Join(netplanDir, name), []byte(content), mode); err != nil {
			restore()
			return err
		}
	}
	for _, name := range plan.Delete {
		remember(name)
		if err := os.Remove(filepath.Join(netplanDir, name)); err != nil && !os.IsNotExist(err) {
			restore()
			return err
		}
	}
	if out, err := exec.Command("netplan", "generate").CombinedOutput(); err != nil {
		restore()
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("netplan: %s", msg)
	}
	stopCloudInitNetwork()
	if out, err := exec.Command("netplan", "apply").CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("netplan apply: %s", msg)
	}
	return nil
}

func stopCloudInitNetwork() {
	if _, err := os.Stat("/etc/cloud"); err != nil {
		if _, err := os.Stat("/usr/bin/cloud-init"); err != nil {
			return
		}
	}
	dir := "/etc/cloud/cloud.cfg.d"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	path := filepath.Join(dir, "99-siroc-network.cfg")
	if b, err := os.ReadFile(path); err == nil && string(b) == cloudInitNetCfg {
		return
	}
	_ = os.WriteFile(path, []byte(cloudInitNetCfg), 0644)
}

func persistNetplanAdd(section, dev, parent string, vlanID int, cidr, gw string) error {
	files, err := readNetplanFiles()
	if err != nil {
		return err
	}
	plan, err := planNetplanAdd(files, section, dev, parent, vlanID, cidr, gw)
	if err != nil {
		return err
	}
	return commitNetplan(plan)
}

func persistNetplanDel(iface, cidr string) error {
	files, err := readNetplanFiles()
	if err != nil {
		return err
	}
	plan, err := planNetplanDel(files, iface, cidr)
	if err != nil {
		return err
	}
	return commitNetplan(plan)
}

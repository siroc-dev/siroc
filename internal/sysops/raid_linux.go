//go:build linux

package sysops

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func ListRAID() (*rpc.RAIDStatus, error) {
	st := readRAID()
	_, err := exec.LookPath("mdadm")
	st.Installed = err == nil
	if !st.Installed {
		st.Message = "mdadm is not installed."
	}
	return st, nil
}

func readRAID() *rpc.RAIDStatus {
	raw, _ := exec.Command("lsblk", "-J", "-b", "-o", "NAME,PATH,TYPE,SIZE,FSTYPE,MOUNTPOINT,MODEL,TRAN").Output()
	md, _ := os.ReadFile("/proc/mdstat")
	st := ClassifyBlockDevices(raw, string(md))
	if st.Arrays == nil {
		st.Arrays = []rpc.RAIDArray{}
	}
	if st.Disks == nil {
		st.Disks = []rpc.RAIDDisk{}
	}
	return st
}

func InstallRAID() (*rpc.RAIDStatus, error) {
	cmd := exec.Command("apt-get", "install", "-y", "mdadm")
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		if len(msg) > 400 {
			msg = msg[len(msg)-400:]
		}
		return nil, fmt.Errorf("install mdadm: %s", msg)
	}
	return ListRAID()
}

func CreateRAID(req rpc.RAIDCreateReq) (*rpc.RAIDStatus, error) {
	if _, err := exec.LookPath("mdadm"); err != nil {
		return nil, fmt.Errorf("mdadm is not installed")
	}
	cur := readRAID()
	level, name, devices, err := CheckRAIDCreate(req.Level, req.Devices, cur.Disks, cur.Arrays)
	if err != nil {
		return nil, err
	}
	if req.Name != "" {
		used := map[string]bool{}
		for _, a := range cur.Arrays {
			used[strings.TrimPrefix(a.Device, "/dev/")] = true
		}
		name, err = PickRAIDName(used, req.Name)
		if err != nil {
			return nil, err
		}
	}
	dev := "/dev/" + name
	args := []string{"--create", dev, "--run", "--level=" + level, fmt.Sprintf("--raid-devices=%d", len(devices))}
	args = append(args, devices...)
	cmd := exec.Command("mdadm", args...)
	done := make(chan error, 1)
	var out []byte
	go func() {
		b, e := cmd.CombinedOutput()
		out = b
		done <- e
	}()
	select {
	case err = <-done:
	case <-time.After(2 * time.Minute):
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("creating the array timed out")
	}
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		if len(msg) > 400 {
			msg = msg[len(msg)-400:]
		}
		return nil, fmt.Errorf("mdadm: %s", msg)
	}
	if err := persistRAID(); err != nil {
		st := readRAID()
		st.Installed = true
		st.Message = "Array created, but it was not saved for boot: " + err.Error()
		return st, nil
	}
	st, err := ListRAID()
	if err != nil {
		return nil, err
	}
	st.Message = "Created " + dev + ". Format it if needed, then mount it below."
	return st, nil
}

func persistRAID() error {
	scan, err := exec.Command("mdadm", "--detail", "--scan").CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(scan))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	dir := "/etc/mdadm"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, "mdadm.conf")
	prev, _ := os.ReadFile(path)
	text := string(prev)
	if !strings.Contains(text, "DEVICE") {
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += "DEVICE partitions\n"
	}
	for _, line := range strings.Split(string(scan), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "ARRAY ") {
			continue
		}
		if strings.Contains(text, line) {
			continue
		}
		if uuid, ok := raidScanUUID(line); ok && strings.Contains(text, uuid) {
			continue
		}
		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += line + "\n"
	}
	return os.WriteFile(path, []byte(text), 0644)
}

func raidScanUUID(line string) (string, bool) {
	for _, f := range strings.Fields(line) {
		key, val, ok := strings.Cut(f, "=")
		if ok && strings.EqualFold(key, "UUID") && val != "" {
			return val, true
		}
	}
	return "", false
}

package diskspace

import (
	"os"
	"strconv"
	"strings"
)

// Mount is one line from df.
type Mount struct {
	Avail int64
	Type  string
	Path  string
}

// Pick returns the mount point with the most free bytes.
// Memory filesystems are skipped so a large tar is not written into RAM.
func Pick(mounts []Mount) string {
	best := ""
	var bestN int64 = -1
	for _, m := range mounts {
		if m.Avail <= bestN || !usableMount(m) {
			continue
		}
		bestN = m.Avail
		best = m.Path
	}
	return best
}

func usableMount(m Mount) bool {
	if m.Avail < 0 || m.Path == "" || strings.Contains(m.Path, "..") {
		return false
	}
	if !strings.HasPrefix(m.Path, "/") || strings.ContainsAny(m.Path, " \t\r\n") {
		return false
	}
	switch strings.ToLower(m.Type) {
	case "tmpfs", "devtmpfs", "squashfs", "overlay", "aufs", "ramfs", "proc", "sysfs",
		"cgroup", "cgroup2", "autofs", "devpts", "securityfs", "pstore", "bpf",
		"tracefs", "debugfs", "fusectl", "configfs", "nsfs", "efivarfs":
		return false
	}
	return true
}

// ParseDF reads `df -PT -B1 --output=avail,fstype,target` without the header.
func ParseDF(text string) []Mount {
	var out []Mount
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Avail") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		avail, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil {
			continue
		}
		out = append(out, Mount{Avail: avail, Type: fields[1], Path: strings.Join(fields[2:], " ")})
	}
	return out
}

func tmpOn(mount string) string {
	mount = strings.TrimRight(mount, "/")
	if mount == "" {
		return "/siroc-tmp"
	}
	return mount + "/siroc-tmp"
}

func tryDir(dir string) bool {
	if err := os.MkdirAll(dir, 0750); err != nil {
		return false
	}
	f, err := os.CreateTemp(dir, ".write-")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

// WritableTmp is a directory on the filesystem with the most free space.
func WritableTmp() (string, error) {
	for _, m := range sorted(readMounts()) {
		if !usableMount(m) {
			continue
		}
		dir := tmpOn(m.Path)
		if tryDir(dir) {
			return dir, nil
		}
	}
	fallback := "/var/tmp/siroc-tmp"
	if tryDir(fallback) {
		return fallback, nil
	}
	return "", os.ErrPermission
}

func sorted(mounts []Mount) []Mount {
	out := append([]Mount(nil), mounts...)
	for i := 1; i < len(out); i++ {
		j := i
		for j > 0 && out[j].Avail > out[j-1].Avail {
			out[j], out[j-1] = out[j-1], out[j]
			j--
		}
	}
	return out
}

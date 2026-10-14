//go:build linux

package sysops

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
)

const exportsPath = "/etc/exports"

func ListNFS() (*rpc.NFSStatus, error) {
	exports, err := readText(exportsPath)
	if err != nil {
		return nil, err
	}
	fstab, err := readText("/etc/fstab")
	if err != nil {
		return nil, err
	}
	proc, _ := readText("/proc/mounts")
	st := BuildNFSStatus(exports, fstab, proc)
	_, err = exec.LookPath("exportfs")
	st.Installed = err == nil
	_, err = exec.LookPath("mount.nfs")
	st.ClientInstalled = err == nil
	st.Active = nfsUnitActive()
	return st, nil
}

func ChangeExport(op string, id int, req rpc.NFSExportReq) (*rpc.NFSStatus, error) {
	text, err := readText(exportsPath)
	if err != nil {
		return nil, err
	}
	next, err := MutateExports(text, op, id, req)
	if err != nil {
		return nil, err
	}
	if err := writeText(exportsPath, next, 0644); err != nil {
		return nil, err
	}
	for _, row := range parseExports(next) {
		if err := os.MkdirAll(row.path, 0755); err != nil {
			return nil, err
		}
	}
	if _, look := exec.LookPath("exportfs"); look != nil {
		st, err := ListNFS()
		if err != nil {
			return nil, err
		}
		st.Message = "Share saved. Install NFS to publish it."
		return st, nil
	}
	if out, err := exec.Command("exportfs", "-ra").CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("share saved, but exportfs failed: %s", msg)
	}
	return ListNFS()
}

func ChangeMount(op string, id int, req rpc.NFSMountReq) (*rpc.NFSStatus, error) {
	fstab, err := readText("/etc/fstab")
	if err != nil {
		return nil, err
	}
	proc, _ := readText("/proc/mounts")
	current := mergeNFSMounts(fstab, proc)
	var prev *rpc.NFSMount
	for i := range current {
		if current[i].ID == id {
			prev = &current[i]
			break
		}
	}
	switch op {
	case "add":
		if err := applyMount(req, nil); err != nil {
			return nil, err
		}
	case "update":
		if prev == nil {
			return nil, fmt.Errorf("NFS mount not found")
		}
		if err := applyMount(req, prev); err != nil {
			return nil, err
		}
	case "delete":
		if prev == nil {
			return nil, fmt.Errorf("NFS mount not found")
		}
		if prev.Mounted {
			if err := umountPath(prev.MountPoint); err != nil {
				return nil, err
			}
		}
		if prev.Persisted {
			next, err := MutateFstabNFS(fstab, "delete", prev.MountPoint, "", "")
			if err != nil {
				return nil, err
			}
			if err := writeText("/etc/fstab", next, 0644); err != nil {
				return nil, err
			}
		}
	case "mount":
		if prev == nil {
			return nil, fmt.Errorf("NFS mount not found")
		}
		opts, err := MountOptionString(mountReq(*prev, true))
		if err != nil {
			return nil, err
		}
		if err := mountNFS(prev.Source, prev.MountPoint, opts); err != nil {
			return nil, err
		}
	case "unmount":
		if prev == nil {
			return nil, fmt.Errorf("NFS mount not found")
		}
		if err := umountPath(prev.MountPoint); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown NFS action")
	}
	return ListNFS()
}

func applyMount(req rpc.NFSMountReq, prev *rpc.NFSMount) error {
	source, mp, err := cleanMountReq(req)
	if err != nil {
		return err
	}
	req.Source, req.MountPoint = source, mp
	opts, err := MountOptionString(req)
	if err != nil {
		return err
	}
	if prev != nil && prev.Mounted && prev.MountPoint != mp {
		if err := umountPath(prev.MountPoint); err != nil {
			return err
		}
	}
	if req.MountNow {
		if mountedPath(mp) {
			if err := umountPath(mp); err != nil {
				return err
			}
		}
		if err := mountNFS(source, mp, opts); err != nil {
			return err
		}
	}
	fstab, err := readText("/etc/fstab")
	if err != nil {
		return err
	}
	if prev != nil && prev.Persisted && prev.MountPoint != mp {
		fstab, err = MutateFstabNFS(fstab, "delete", prev.MountPoint, "", "")
		if err != nil {
			return err
		}
	}
	if req.Persist {
		fstab, err = MutateFstabNFS(fstab, "add", mp, source, opts)
		if err != nil {
			return err
		}
		return writeText("/etc/fstab", fstab, 0644)
	}
	if prev != nil && prev.Persisted {
		fstab, err = MutateFstabNFS(fstab, "delete", mp, "", "")
		if err != nil {
			return err
		}
		return writeText("/etc/fstab", fstab, 0644)
	}
	return nil
}

func mountReq(row rpc.NFSMount, now bool) rpc.NFSMountReq {
	return rpc.NFSMountReq{
		Source: row.Source, MountPoint: row.MountPoint, Options: row.Options,
		NFSVers: row.NFSVers, Proto: row.Proto, Timeo: row.Timeo, Retrans: row.Retrans,
		RSize: row.RSize, WSize: row.WSize, Persist: row.Persisted, MountNow: now,
	}
}

func mountNFS(source, mp, opts string) error {
	if err := os.MkdirAll(mp, 0755); err != nil {
		return err
	}
	out, err := exec.Command("mount", "-t", "nfs", "-o", opts, source, mp).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("mount: %s", msg)
	}
	return nil
}

func umountPath(mp string) error {
	if mp == "" || mp == "/" {
		return fmt.Errorf("invalid mount point")
	}
	out, err := exec.Command("umount", mp).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("umount: %s", msg)
	}
	return nil
}

func mountedPath(mp string) bool {
	b, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[1] == mp {
			return true
		}
	}
	return false
}

func InstallNFS() (*rpc.NFSStatus, error) {
	cmd := exec.Command("apt-get", "install", "-y", "nfs-kernel-server", "nfs-common")
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("install NFS: %s", tailText(string(out)))
	}
	_ = nfsctl("enable", "--now")
	return ListNFS()
}

func ControlNFS(action string) (*rpc.NFSStatus, error) {
	switch action {
	case "start", "stop", "restart":
	default:
		return nil, fmt.Errorf("unknown NFS action")
	}
	if err := nfsctl(action); err != nil {
		return nil, err
	}
	return ListNFS()
}

func nfsctl(args ...string) error {
	unit := "nfs-server"
	if exec.Command("systemctl", "cat", unit).Run() != nil {
		unit = "nfs-kernel-server"
	}
	cmd := append(args, unit)
	out, err := exec.Command("systemctl", cmd...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("nfs service: %s", msg)
	}
	return nil
}

func nfsUnitActive() bool {
	if exec.Command("systemctl", "is-active", "--quiet", "nfs-server").Run() == nil {
		return true
	}
	return exec.Command("systemctl", "is-active", "--quiet", "nfs-kernel-server").Run() == nil
}

func readText(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(b), nil
}

func writeText(path, text string, mode os.FileMode) error {
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".nfs-*")
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
	if err := f.Chmod(mode); err != nil {
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

func tailText(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		s = s[len(s)-500:]
	}
	return s
}

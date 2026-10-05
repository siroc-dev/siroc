//go:build linux

package osupdate

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

const (
	stateFile = "/var/lib/siroc/os-updates.json"
	lockFile  = "/var/lib/siroc/os-updates.lock"
	checkUnit = "siroc-os-update"
	checkCal  = "*-*-* 04:15:00"
)

func Saved() rpc.OSUpdateStatus {
	b, err := os.ReadFile(stateFile)
	if err != nil {
		return rpc.OSUpdateStatus{
			OK:      true,
			Message: "Not checked yet. Update refreshes the package lists. Siroc checks every day at 04:15.",
		}
	}
	var st rpc.OSUpdateStatus
	if json.Unmarshal(b, &st) != nil {
		return rpc.OSUpdateStatus{OK: true, Message: "The saved package list could not be read. Run Update again."}
	}
	if st.Packages == nil {
		st.Packages = []rpc.OSPackage{}
	}
	st.OK = true
	st.Count = len(st.Packages)
	return st
}

func Update() (rpc.OSUpdateStatus, error) {
	var st rpc.OSUpdateStatus
	err := locked(func() error {
		var err error
		st, err = refresh(false)
		return err
	})
	return st, err
}

func Upgrade() (rpc.OSUpdateStatus, error) {
	var st rpc.OSUpdateStatus
	err := locked(func() error {
		if _, err := exec.LookPath("apt-get"); err != nil {
			return fmt.Errorf("apt-get is not installed")
		}
		if _, err := runApt("update", "-qq"); err != nil {
			return err
		}
		out, upErr := runApt("-y", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold", "upgrade")
		listed, listErr := collect()
		st = listed
		st.Log = tail(out, 4000)
		if upErr != nil {
			if st.Message == "" {
				st.Message = "Upgrade failed."
			}
			st.Message = "Upgrade failed. " + upErr.Error()
			_ = save(st)
			return upErr
		}
		if listErr != nil {
			return listErr
		}
		if st.Count == 0 {
			st.Message = "Upgrade finished. No packages left to upgrade."
		} else {
			st.Message = "Upgrade finished. " + Summarize(st.Count)
		}
		return save(st)
	})
	return st, err
}

func InstallTimer() error {
	bin, err := os.Executable()
	if err != nil || bin == "" {
		bin = "/usr/local/bin/siroc-agent"
	}
	if resolved, err := filepath.EvalSymlinks(bin); err == nil && resolved != "" {
		bin = resolved
	}
	svc := fmt.Sprintf(`[Unit]
Description=Refresh the apt package list for Siroc
After=network-online.target

[Service]
Type=oneshot
Environment=HOME=/root
Environment=DEBIAN_FRONTEND=noninteractive
ExecStart=%s os-check
`, bin)
	timer := `[Unit]
Description=Daily OS package check at 04:15

[Timer]
OnCalendar=` + checkCal + `
Persistent=true

[Install]
WantedBy=timers.target
`
	if err := os.WriteFile("/etc/systemd/system/"+checkUnit+".service", []byte(svc), 0644); err != nil {
		return err
	}
	if err := os.WriteFile("/etc/systemd/system/"+checkUnit+".timer", []byte(timer), 0644); err != nil {
		return err
	}
	_ = exec.Command("systemctl", "daemon-reload").Run()
	if err := exec.Command("systemctl", "enable", "--now", checkUnit+".timer").Run(); err != nil {
		return fmt.Errorf("enable %s.timer: %w", checkUnit, err)
	}
	return nil
}

func refresh(keepLog bool) (rpc.OSUpdateStatus, error) {
	if _, err := exec.LookPath("apt-get"); err != nil {
		return rpc.OSUpdateStatus{}, fmt.Errorf("apt-get is not installed")
	}
	if _, err := runApt("update", "-qq"); err != nil {
		return Saved(), err
	}
	st, err := collect()
	if err != nil {
		return Saved(), err
	}
	if keepLog {
		st.Log = Saved().Log
	}
	if err := save(st); err != nil {
		return st, err
	}
	return st, nil
}

func collect() (rpc.OSUpdateStatus, error) {
	listOut, err := runCmd("apt", "list", "--upgradable")
	if err != nil && !strings.Contains(listOut, "upgradable from") && !strings.Contains(listOut, "Listing") {
		return rpc.OSUpdateStatus{}, err
	}
	pkgs := ParseUpgradable(listOut)
	return rpc.OSUpdateStatus{
		OK:        true,
		OS:        prettyOS(),
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
		Count:     len(pkgs),
		Packages:  pkgs,
		Message:   Summarize(len(pkgs)),
	}, nil
}

func save(st rpc.OSUpdateStatus) error {
	if st.Packages == nil {
		st.Packages = []rpc.OSPackage{}
	}
	st.Count = len(st.Packages)
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll("/var/lib/siroc", 0755); err != nil {
		return err
	}
	return os.WriteFile(stateFile, append(b, '\n'), 0644)
}

func locked(fn func() error) error {
	if err := os.MkdirAll("/var/lib/siroc", 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}

func runApt(args ...string) (string, error) {
	return runCmd("apt-get", args...)
}

func runCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(),
		"DEBIAN_FRONTEND=noninteractive",
		"LANG=C",
		"LC_ALL=C",
		"NEEDRESTART_MODE=a",
		"APT_LISTCHANGES_FRONTEND=none",
	)
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err != nil {
		return text, fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), tail(text, 500))
	}
	return text, nil
}

func prettyOS() string {
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"'`)
		}
	}
	return ""
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

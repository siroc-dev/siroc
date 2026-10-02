//go:build linux

package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	checkUnit = "siroc-update-check"
	checkCal  = "*-*-* 04:00:00"
)

func InstallCheckTimer() error {
	bin, err := os.Executable()
	if err != nil {
		bin = "/usr/local/bin/siroc-agent"
	}
	bin, _ = filepath.EvalSymlinks(bin)
	if bin == "" {
		bin = "/usr/local/bin/siroc-agent"
	}
	svc := fmt.Sprintf(`[Unit]
Description=Check Siroc updates from the public channel
After=network-online.target

[Service]
Type=oneshot
Environment=HOME=/root
Environment=SIROC_UPDATE_URL=%s
ExecStart=%s update
`, DefaultChannel, bin)
	timer := `[Unit]
Description=Daily Siroc update check at 04:00

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

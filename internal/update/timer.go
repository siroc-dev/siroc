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
	checkCal  = "hourly"
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
ExecStart=%s update --apply
`, DefaultChannel, bin)
	timer := `[Unit]
Description=Hourly Siroc update check and install

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
	if !AutoEnabled() {
		_ = exec.Command("systemctl", "disable", "--now", checkUnit+".timer").Run()
		return nil
	}
	if err := exec.Command("systemctl", "enable", "--now", checkUnit+".timer").Run(); err != nil {
		return fmt.Errorf("enable %s.timer: %w", checkUnit, err)
	}
	return nil
}

// SetAutoUpdate remembers the switch and starts or stops the hourly installer.
func SetAutoUpdate(on bool) error {
	if err := writeAuto(on); err != nil {
		return err
	}
	return InstallCheckTimer()
}

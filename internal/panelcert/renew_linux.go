//go:build linux

package panelcert

import (
	"fmt"
	"os"
	"os/exec"
)

const (
	renewServicePath = "/etc/systemd/system/siroc-panel-cert.service"
	renewTimerPath   = "/etc/systemd/system/siroc-panel-cert.timer"
)

// InstallRenewTimer writes the daily renewal units and enables them.
// A machine without systemd returns the error to the caller, which keeps the
// certificate that was just issued.
func InstallRenewTimer(dataDir string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	service := RenewService(dataDir, exe)
	timer := RenewTimer()
	if err := os.WriteFile(renewServicePath, []byte(service), 0644); err != nil {
		return fmt.Errorf("write %s: %w", renewServicePath, err)
	}
	if err := os.WriteFile(renewTimerPath, []byte(timer), 0644); err != nil {
		return fmt.Errorf("write %s: %w", renewTimerPath, err)
	}
	if out, err := exec.Command("systemctl", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("reload systemd: %s", out)
	}
	if out, err := exec.Command("systemctl", "enable", "--now", "siroc-panel-cert.timer").CombinedOutput(); err != nil {
		return fmt.Errorf("enable certificate timer: %s", out)
	}
	return nil
}

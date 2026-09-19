//go:build linux

package weblog

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func InstallTimers() error {
	bin, err := os.Executable()
	if err != nil {
		bin = "/usr/local/bin/siroc-agent"
	}
	bin, _ = filepath.EvalSymlinks(bin)
	if bin == "" {
		bin = "/usr/local/bin/siroc-agent"
	}
	units := []struct {
		name    string
		service string
		timer   string
	}{
		{
			name:    "siroc-cloudflare-ips",
			service: "[Unit]\nDescription=Refresh Cloudflare IP ranges for nginx real_ip\nAfter=network-online.target\n\n[Service]\nType=oneshot\nExecStart=%s cloudflare-ips\n",
			timer:   "[Unit]\nDescription=Daily Cloudflare IP refresh for nginx\n\n[Timer]\nOnCalendar=*-*-* 03:20:00\nPersistent=true\nRandomizedDelaySec=10m\n\n[Install]\nWantedBy=timers.target\n",
		},
		{
			name:    "siroc-weblog",
			service: "[Unit]\nDescription=Rotate Siroc web logs and rebuild GoAccess reports\nAfter=network.target\n\n[Service]\nType=oneshot\nExecStart=%s weblog\n",
			timer:   "[Unit]\nDescription=Daily Siroc log rotate and GoAccess\n\n[Timer]\nOnCalendar=*-*-* 04:10:00\nPersistent=true\nRandomizedDelaySec=10m\n\n[Install]\nWantedBy=timers.target\n",
		},
	}
	for _, u := range units {
		svc := fmt.Sprintf(u.service, bin)
		if err := os.WriteFile("/etc/systemd/system/"+u.name+".service", []byte(svc), 0644); err != nil {
			return err
		}
		if err := os.WriteFile("/etc/systemd/system/"+u.name+".timer", []byte(u.timer), 0644); err != nil {
			return err
		}
		_ = exec.Command("systemctl", "daemon-reload").Run()
		if err := exec.Command("systemctl", "enable", "--now", u.name+".timer").Run(); err != nil {
			return fmt.Errorf("enable %s.timer: %w", u.name, err)
		}
	}
	return nil
}

//go:build linux

package backup

import (
	"os"
	"strings"
)

const cronPath = "/etc/cron.d/siroc-backup"

// InstallCron replaces the managed backup crontab. The body must be a Siroc cron file.
func InstallCron(body string) error {
	if err := ValidCronFile(body); err != nil {
		return err
	}
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	if err := os.MkdirAll("/var/log/siroc", 0750); err != nil {
		return err
	}
	if err := os.MkdirAll("/etc/cron.d", 0755); err != nil {
		return err
	}
	tmp := cronPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(body), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, cronPath)
}

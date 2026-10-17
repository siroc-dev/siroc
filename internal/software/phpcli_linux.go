//go:build linux

package software

import (
	"os"
	"path/filepath"
)

// EnsurePHPCLILimits writes CLI-only limits for every installed PHP version.
// Web requests keep using PHP-FPM's php.ini and pool settings.
func EnsurePHPCLILimits() error {
	dirs, err := filepath.Glob("/etc/php/*/cli")
	if err != nil {
		return err
	}
	var first error
	body := []byte(CLILimitsINI())
	for _, dir := range dirs {
		confd := filepath.Join(dir, "conf.d")
		if err := os.MkdirAll(confd, 0755); err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		path := filepath.Join(confd, "99-siroc-cli.ini")
		cur, err := os.ReadFile(path)
		if err == nil && string(cur) == string(body) {
			continue
		}
		if err := os.WriteFile(path, body, 0644); err != nil && first == nil {
			first = err
		}
	}
	return first
}

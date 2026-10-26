//go:build linux

package diskspace

import (
	"os/exec"
	"time"
)

func readMounts() []Mount {
	cmd := exec.Command("df", "-PT", "-B1", "--output=avail,fstype,target")
	timer := time.AfterFunc(10*time.Second, func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	defer timer.Stop()
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	return ParseDF(string(out))
}

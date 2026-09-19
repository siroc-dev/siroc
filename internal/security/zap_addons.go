//go:build linux

package security

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	zapHomeDir     = "/var/lib/siroc/zap"
	zapAddonStamp  = "/var/lib/siroc/zap/.addons-updated"
	zapAddonMaxAge = 7 * 24 * time.Hour
)

// UpdateZAPAddons refreshes Marketplace add-ons so scans are not stuck on
// rules that have not been checked for months.
func UpdateZAPAddons() error {
	bin := zapBin()
	if _, err := os.Stat(bin); err != nil {
		if p, look := exec.LookPath("zap.sh"); look == nil {
			bin = p
		} else {
			return fmt.Errorf("OWASP ZAP is not installed")
		}
	}
	if err := os.MkdirAll(zapHomeDir, 0750); err != nil {
		return err
	}
	port, err := freeLocalPort()
	if err != nil {
		port = 18081
	}
	portS := strconv.Itoa(port)
	cmd := exec.Command(bin,
		"-cmd",
		"-notel",
		"-host", "127.0.0.1",
		"-port", portS,
		"-dir", zapHomeDir,
		"-addonupdate",
	)
	cmd.Env = append(os.Environ(), "HOME="+zapHomeDir, "ZAP_PORT="+portS)
	out, err := combinedScan(cmd, 10*time.Minute)
	if err != nil {
		return fmt.Errorf("ZAP add-on update failed: %s", tailScan(out))
	}
	_ = os.WriteFile(zapAddonStamp, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0640)
	markZAPCheckedToday()
	return nil
}

func ensureZAPAddons() {
	if st, err := os.Stat(zapAddonStamp); err == nil && time.Since(st.ModTime()) < zapAddonMaxAge {
		markZAPCheckedToday()
		return
	}
	_ = UpdateZAPAddons()
}

func zapCheckedToday() string {
	return time.Now().UTC().Format("2006-01-02")
}

func markZAPCheckedToday() {
	today := zapCheckedToday()
	path := filepath.Join(zapHomeDir, "config.xml")
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	conf := string(b)
	re := regexp.MustCompile(`<dayLastChecked>[^<]*</dayLastChecked>`)
	if re.MatchString(conf) {
		conf = re.ReplaceAllString(conf, "<dayLastChecked>"+today+"</dayLastChecked>")
	} else if i := strings.Index(conf, "</config>"); i >= 0 {
		block := "    <start>\n        <dayLastChecked>" + today + "</dayLastChecked>\n    </start>\n"
		conf = conf[:i] + block + conf[i:]
	} else {
		return
	}
	_ = os.WriteFile(path, []byte(conf), 0644)
}

//go:build linux

package security

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const clamScanHelper = "/opt/siroc/bin/siroc-clamscan"

func writeClamScanHelper() error {
	if err := os.MkdirAll("/opt/siroc/bin", 0755); err != nil {
		return err
	}
	script := `#!/bin/bash
set -eu
f="${1:-}"
if [ -z "$f" ] || [ ! -e "$f" ]; then
  echo 0
  exit 0
fi
out=""
if command -v clamdscan >/dev/null 2>&1 && systemctl is-active --quiet clamav-daemon; then
  out=$(clamdscan --fdpass --no-summary --stdout -- "$f" 2>/dev/null || true)
else
  out=$(clamscan --no-summary --stdout --max-filesize=25M -- "$f" 2>/dev/null || true)
fi
if printf '%s\n' "$out" | grep -q ' FOUND$'; then
  echo "1 $out"
  exit 0
fi
echo 0
exit 0
`
	if err := os.WriteFile(clamScanHelper, []byte(script), 0755); err != nil {
		return err
	}
	return nil
}

func uploadScanRule() string {
	return `SecRule FILES_TMPNAMES "@inspectFile ` + clamScanHelper + `" "id:1999001,phase:2,deny,status:403,t:none,msg:'Malware detected in uploaded file',log,auditlog"
`
}

func UploadScanEnabled() bool {
	return loadWAFState().UploadScan
}

func ScanUpload(path string) error {
	if !UploadScanEnabled() {
		return nil
	}
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	bin := "clamscan"
	args := []string{"--no-summary", "--infected", "--max-filesize=25M", path}
	if _, err := exec.LookPath("clamdscan"); err == nil && exec.Command("systemctl", "is-active", "--quiet", "clamav-daemon").Run() == nil {
		bin = "clamdscan"
		args = []string{"--fdpass", "--no-summary", "--infected", path}
	} else if _, err := exec.LookPath("clamscan"); err != nil {
		return nil
	}
	cmd := exec.Command(bin, args...)
	out, err := combinedTimeout(cmd, 2*time.Minute)
	if countInfected(out) > 0 || strings.Contains(out, " FOUND") {
		_ = os.Remove(path)
		return fmt.Errorf("upload blocked: malware detected")
	}
	if err != nil && !strings.Contains(out, "Infected files") && !strings.Contains(out, "OK") {
		return nil
	}
	return nil
}

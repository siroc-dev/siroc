//go:build linux

package software

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/security"
)

func niktoInstalled() (bool, string) {
	return binVersion("nikto")
}

func zapInstalled() (bool, string) {
	if matches, _ := filepath.Glob("/opt/zaproxy/zap-*.jar"); len(matches) > 0 {
		base := filepath.Base(matches[0])
		ver := strings.TrimSuffix(strings.TrimPrefix(base, "zap-"), ".jar")
		return true, "OWASP ZAP " + ver
	}
	if fileOKZap("/opt/zaproxy/zap.sh") {
		return true, "OWASP ZAP"
	}
	if _, err := exec.LookPath("zap.sh"); err == nil {
		return true, "OWASP ZAP"
	}
	return false, ""
}

func openvasInstalled() (bool, string) {
	if p, err := exec.LookPath("gvm-cli"); err == nil {
		out, _ := exec.Command(p, "--version").CombinedOutput()
		if line := firstLine(string(out)); line != "" {
			return true, line
		}
		return true, "gvm-cli"
	}
	if ok, ver := dpkgVersion("gvmd"); ok {
		return true, ver
	}
	if ok, ver := dpkgVersion("openvas"); ok {
		return true, ver
	}
	if _, err := exec.LookPath("gvmd"); err == nil {
		return true, "gvmd"
	}
	return false, ""
}

func fileOKZap(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	return s
}

func installNikto() error {
	if err := aptInstall("nikto"); err != nil {
		return err
	}
	return nil
}

func installZAP() error {
	if err := aptInstall("default-jre-headless", "wget", "tar"); err != nil {
		return err
	}
	if matches, _ := filepath.Glob("/opt/zaproxy/zap-2.17*.jar"); len(matches) > 0 && fileOKZap("/opt/zaproxy/zap.sh") {
		_ = os.Chmod("/opt/zaproxy/zap.sh", 0755)
		_ = os.Remove("/usr/local/bin/zap.sh")
		_ = os.Symlink("/opt/zaproxy/zap.sh", "/usr/local/bin/zap.sh")
		_ = security.UpdateZAPAddons()
		return nil
	}
	tmp := filepath.Join(ensureDiskTmp(), "zap")
	_ = os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0755); err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	tgz := filepath.Join(tmp, "zap.tgz")
	urls := []string{
		"https://github.com/zaproxy/zaproxy/releases/download/v2.17.0/ZAP_2.17.0_Linux.tar.gz",
		"https://github.com/zaproxy/zaproxy/releases/download/v2.16.1/ZAP_2.16.1_Linux.tar.gz",
	}
	var last error
	ok := false
	for _, u := range urls {
		dl := exec.Command("curl", "-fsSL", "-L", "-o", tgz, u)
		if out, err := combinedTimeout(dl, 4*time.Minute); err != nil {
			last = fmt.Errorf("download zap: %s", tail(out))
			continue
		}
		ok = true
		break
	}
	if !ok {
		if last == nil {
			last = fmt.Errorf("download zap failed")
		}
		return last
	}
	if out, err := combinedTimeout(exec.Command("tar", "-xzf", tgz, "-C", tmp), 2*time.Minute); err != nil {
		return fmt.Errorf("extract zap: %s", tail(out))
	}
	_ = os.RemoveAll("/opt/zaproxy")
	matches, _ := filepath.Glob(filepath.Join(tmp, "ZAP_*"))
	src := ""
	for _, m := range matches {
		if st, err := os.Stat(m); err == nil && st.IsDir() {
			src = m
			break
		}
	}
	if src == "" {
		return fmt.Errorf("zap archive did not contain a ZAP directory")
	}
	if err := exec.Command("cp", "-a", src, "/opt/zaproxy").Run(); err != nil {
		return fmt.Errorf("install zap files: %w", err)
	}
	_ = os.Chmod("/opt/zaproxy/zap.sh", 0755)
	_ = exec.Command("touch", "-c", "/opt/zaproxy/zap.sh").Run()
	_ = os.Remove("/usr/local/bin/zap.sh")
	_ = os.Symlink("/opt/zaproxy/zap.sh", "/usr/local/bin/zap.sh")
	_ = security.UpdateZAPAddons()
	return nil
}

func installOpenVAS() error {
	_ = aptInstall("postgresql", "postgresql-contrib", "rsync", "gnutls-bin", "xmlstarlet", "python3-gvm", "nmap")
	var last error
	for _, group := range [][]string{
		{"gvm", "gvm-tools", "gvmd", "gsad", "ospd-openvas", "openvas-scanner", "greenbone-feed-sync"},
		{"openvas", "gvm-tools"},
		{"gvmd", "gvm-tools", "gsad"},
	} {
		if err := aptInstall(group...); err != nil {
			last = err
			continue
		}
		last = nil
		break
	}
	if last != nil && !openvasPresent() {
		return fmt.Errorf("OpenVAS/GVM packages are not available on this distro: %w", last)
	}
	if err := security.PrepareOpenVAS(); err != nil {
		return err
	}
	if _, err := exec.LookPath("greenbone-feed-sync"); err == nil {
		cmd := exec.Command("greenbone-feed-sync")
		cmd.Env = append(os.Environ(), "HOME=/var/lib/gvm")
		go func() { _, _ = combinedTimeout(cmd, 40*time.Minute) }()
	}
	return nil
}

func openvasPresent() bool {
	ok, _ := openvasInstalled()
	return ok
}

//go:build linux

package security

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func PinGvmTemp() {
	dir := "/var/tmp/gvm"
	_ = os.MkdirAll(dir, 0755)
	_ = exec.Command("chown", "_gvm:_gvm", dir).Run()
	drop := "[Service]\nEnvironment=TMPDIR=/var/tmp/gvm\nEnvironment=TMP=/var/tmp/gvm\nEnvironment=TEMP=/var/tmp/gvm\n"
	for _, svc := range []string{"gvmd", "gsad", "ospd-openvas"} {
		d := "/etc/systemd/system/" + svc + ".service.d"
		_ = os.MkdirAll(d, 0755)
		_ = os.WriteFile(filepath.Join(d, "siroc-tmp.conf"), []byte(drop), 0644)
	}
	matches, _ := filepath.Glob("/tmp/gvmd-split-xml-file-*")
	for _, m := range matches {
		_ = os.RemoveAll(m)
	}
	_ = exec.Command("systemctl", "daemon-reload").Run()
	if exec.Command("systemctl", "is-active", "--quiet", "gvmd").Run() == nil {
		_ = exec.Command("systemctl", "restart", "gvmd").Run()
	}
}

func PrepareOpenVAS() error {
	PinGvmTemp()
	_ = exec.Command("systemctl", "enable", "--now", "postgresql").Run()
	if err := ensureGVMDatabase(); err != nil {
		return err
	}
	if err := ensureGVMCerts(); err != nil {
		return err
	}
	_ = exec.Command("systemctl", "enable", "--now", "redis-server@openvas").Run()
	_ = exec.Command("systemctl", "enable", "--now", "ospd-openvas").Run()
	_ = exec.Command("systemctl", "reset-failed", "gvmd", "gsad").Run()
	_ = exec.Command("systemctl", "enable", "--now", "gvmd").Run()
	_ = exec.Command("systemctl", "restart", "gvmd").Run()
	if err := waitGVMSocket(45 * time.Second); err != nil {
		return err
	}
	_ = exec.Command("systemctl", "enable", "--now", "gsad").Run()
	_ = exec.Command("systemctl", "restart", "gsad").Run()
	ensureGVMUser()
	ensureFeedOwner()
	ensureScanConfigs()
	startFeedSyncIfNeeded()
	return nil
}

const feedImportOwnerUUID = "78eceaec-3385-11ea-b237-28d24461215b"

func ensureFeedOwner() {
	raw, err := exec.Command("runuser", "-u", "_gvm", "--", "gvmd", "--get-users", "--verbose").CombinedOutput()
	if err != nil {
		return
	}
	uid := uuidRe.FindString(string(raw))
	if uid == "" {
		return
	}
	_ = exec.Command("runuser", "-u", "_gvm", "--", "gvmd", "--modify-setting", feedImportOwnerUUID, "--value", uid).Run()
}

func ensureScanConfigs() {
	sock := gvmSocket()
	if sock == "" {
		return
	}
	cfg, _ := gvmXML(sock, openvasPassword(), `<get_configs/>`)
	if strings.Contains(cfg, "Full and fast") {
		return
	}
	_ = exec.Command("runuser", "-u", "_gvm", "--", "gvmd", "--rebuild-gvmd-data=configs,port_lists").Run()
}

func startFeedSyncIfNeeded() {
	if _, err := exec.LookPath("greenbone-feed-sync"); err != nil {
		return
	}
	if feedSyncRunning() {
		return
	}
	if pluginCount() > 50 {
		return
	}
	cmd := exec.Command("greenbone-feed-sync")
	cmd.Env = append(os.Environ(), "HOME=/var/lib/gvm")
	go func() { _, _ = combinedScan(cmd, 90*time.Minute) }()
}

func feedSyncRunning() bool {
	out, _ := exec.Command("bash", "-c", "ps -eo args= | grep -F greenbone-feed-sync | grep -v grep").CombinedOutput()
	return strings.TrimSpace(string(out)) != ""
}

func pluginCount() int {
	n := 0
	_ = filepath.WalkDir("/var/lib/openvas/plugins", func(_ string, d os.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil
		}
		if !d.IsDir() {
			n++
			if n > 50 {
				return filepath.SkipAll
			}
		}
		return nil
	})
	return n
}

func ensureGVMDatabase() error {
	out, err := exec.Command("runuser", "-u", "postgres", "--", "psql", "-tAc", `SELECT 1 FROM pg_roles WHERE rolname='_gvm'`).CombinedOutput()
	if err == nil && strings.TrimSpace(string(out)) == "1" {
		out2, err2 := exec.Command("runuser", "-u", "postgres", "--", "psql", "-tAc", `SELECT 1 FROM pg_database WHERE datname='gvmd'`).CombinedOutput()
		if err2 == nil && strings.TrimSpace(string(out2)) == "1" {
			return nil
		}
	}
	script := "/usr/share/gvm/create-postgresql-database"
	if _, err := os.Stat(script); err != nil {
		return createGVMDatabaseFallback()
	}
	cmd := exec.Command("runuser", "-u", "postgres", "--", script)
	raw, err := combinedScan(cmd, 2*time.Minute)
	if err != nil {
		if fb := createGVMDatabaseFallback(); fb != nil {
			return fmt.Errorf("create GVM database: %s: %w", tailScan(raw), err)
		}
	}
	return nil
}

func createGVMDatabaseFallback() error {
	cmds := [][]string{
		{"createuser", "-DRS", "_gvm"},
		{"createdb", "-O", "_gvm", "gvmd"},
	}
	for _, args := range cmds {
		_ = exec.Command("runuser", append([]string{"-u", "postgres", "--"}, args...)...).Run()
	}
	sql := []string{
		`create role dba with superuser noinherit`,
		`grant dba to _gvm`,
		`create extension if not exists "uuid-ossp"`,
		`create extension if not exists "pgcrypto"`,
		`create extension if not exists "pg-gvm"`,
	}
	for _, q := range sql {
		_ = exec.Command("runuser", "-u", "postgres", "--", "psql", "-c", q, "gvmd").Run()
	}
	out, err := exec.Command("runuser", "-u", "postgres", "--", "psql", "-tAc", `SELECT 1 FROM pg_roles WHERE rolname='_gvm'`).CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "1" {
		return fmt.Errorf("PostgreSQL role _gvm was not created")
	}
	return nil
}

func ensureGVMCerts() error {
	key := "/var/lib/gvm/private/CA/serverkey.pem"
	if st, err := os.Stat(key); err == nil && st.Size() > 0 {
		return nil
	}
	if _, err := exec.LookPath("gvm-manage-certs"); err != nil {
		return fmt.Errorf("gvm-manage-certs is missing")
	}
	raw, err := combinedScan(exec.Command("gvm-manage-certs", "-a"), 45*time.Second)
	_ = exec.Command("chown", "-R", "_gvm:_gvm", "/var/lib/gvm/CA", "/var/lib/gvm/private").Run()
	if _, stat := os.Stat(key); stat != nil {
		return fmt.Errorf("could not create Greenbone TLS certs: %s", tailScan(raw+" "+errString(err)))
	}
	return nil
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func waitGVMSocket(d time.Duration) error {
	deadline := time.Now().Add(d)
	var last string
	for time.Now().Before(deadline) {
		if p := gvmSocket(); p != "" {
			return nil
		}
		last = gvmLogTail()
		time.Sleep(2 * time.Second)
	}
	if last == "" {
		last = "gvmd did not create /run/gvmd/gvmd.sock"
	}
	return fmt.Errorf("gvmd socket not found after starting services: %s", last)
}

func gvmLogTail() string {
	b, err := os.ReadFile("/var/log/gvm/gvmd.log")
	if err != nil {
		return ""
	}
	return tailScan(string(b))
}

func ensureGVMUser() {
	pass := openvasPassword()
	_ = os.MkdirAll(filepath.Dir("/var/lib/siroc/openvas.pass"), 0750)
	_ = os.WriteFile("/var/lib/siroc/openvas.pass", []byte(pass+"\n"), 0640)
	_ = exec.Command("runuser", "-u", "_gvm", "--", "gvmd", "--create-user=admin", "--password="+pass).Run()
}

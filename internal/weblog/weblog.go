//go:build linux

package weblog

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/validate"
)

const (
	retentionPath   = "/var/lib/siroc/log-retention"
	logrotatePath   = "/etc/logrotate.d/siroc"
	nginxLogDir     = "/var/log/nginx/sites"
	apacheLogDir    = "/var/log/apache2/sites"
	goaccessDir     = "/var/lib/siroc/goaccess"
	defaultRetain   = 90
	minRetain       = 1
	maxRetain       = 3650
)

func Ensure() {
	_ = os.MkdirAll("/var/lib/siroc", 0750)
	if _, err := os.Stat(retentionPath); err != nil {
		_ = os.WriteFile(retentionPath, []byte(strconv.Itoa(defaultRetain)+"\n"), 0644)
	}
	_ = InstallTimers()
	_ = WriteLogrotate(Retention())
	_ = EnsureSiteLogs()
	go func() {
		if _, err := os.Stat("/usr/sbin/nginx"); err == nil {
			if err := UpdateCloudflare(false); err != nil {
				fmt.Fprintf(os.Stderr, "siroc-agent: cloudflare real ip: %v\n", err)
			}
		}
		ensureLogrotatePkg()
		ensureGoAccessPkg()
	}()
}

func ApplyNginx() error {
	_ = EnsureLogDirs()
	if err := UpdateCloudflare(true); err != nil {
		return err
	}
	return EnsureSiteLogs()
}

func Daily() error {
	var first error
	if err := Rotate(); err != nil {
		first = err
	}
	if err := GenerateAll(); err != nil && first == nil {
		first = err
	}
	return first
}

func Status() rpc.LogStatus {
	prefixes, updated, source := cloudflareStatus()
	_, ga := exec.LookPath("goaccess")
	_, lr := exec.LookPath("logrotate")
	_, ngx := os.Stat("/usr/sbin/nginx")
	return rpc.LogStatus{
		RetentionDays:        Retention(),
		CloudflarePrefixes:   prefixes,
		CloudflareUpdated:    updated,
		CloudflareSource:     source,
		CloudflareOK:         prefixes > 0 && ngx == nil,
		RealIPPath:           realIPPath,
		GoAccess:             ga == nil,
		Logrotate:            lr == nil,
		NginxLogs:            nginxLogDir,
		RealIPModule:         nginxHasRealIP(),
	}
}

func Retention() int {
	b, err := os.ReadFile(retentionPath)
	if err != nil {
		return defaultRetain
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return defaultRetain
	}
	return clampRetain(n)
}

func SetRetention(days int) error {
	days = clampRetain(days)
	if err := os.MkdirAll("/var/lib/siroc", 0750); err != nil {
		return err
	}
	if err := os.WriteFile(retentionPath, []byte(strconv.Itoa(days)+"\n"), 0644); err != nil {
		return err
	}
	return WriteLogrotate(days)
}

func clampRetain(n int) int {
	if n < minRetain {
		return defaultRetain
	}
	if n > maxRetain {
		return maxRetain
	}
	return n
}

func EnsureLogDirs() error {
	for _, d := range []string{nginxLogDir, apacheLogDir, goaccessDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	_ = exec.Command("chown", "www-data:adm", nginxLogDir).Run()
	_ = exec.Command("chown", "root:adm", apacheLogDir).Run()
	_ = os.Chmod(nginxLogDir, 0755)
	_ = os.Chmod(apacheLogDir, 0755)
	return nil
}

func TouchSiteLogs(domain string) error {
	if err := validate.Domain(domain); err != nil {
		return err
	}
	if err := EnsureLogDirs(); err != nil {
		return err
	}
	files := []struct {
		path  string
		owner string
	}{
		{filepath.Join(nginxLogDir, domain+"-access.log"), "www-data:adm"},
		{filepath.Join(nginxLogDir, domain+"-error.log"), "www-data:adm"},
		{filepath.Join(apacheLogDir, domain+"-access.log"), "root:adm"},
		{filepath.Join(apacheLogDir, domain+"-error.log"), "root:adm"},
	}
	for _, f := range files {
		fh, err := os.OpenFile(f.path, os.O_CREATE|os.O_APPEND, 0640)
		if err != nil {
			return err
		}
		_ = fh.Close()
		_ = exec.Command("chown", f.owner, f.path).Run()
	}
	return nil
}

func NginxAccessLog(domain string) string {
	return filepath.Join(nginxLogDir, domain+"-access.log")
}

func NginxErrorLog(domain string) string {
	return filepath.Join(nginxLogDir, domain+"-error.log")
}

func WriteLogrotate(days int) error {
	days = clampRetain(days)
	if err := EnsureLogDirs(); err != nil {
		return err
	}
	body := fmt.Sprintf(`# Managed by siroc. Retention: %d days.
%s/*.log
%s/*.log
{
    daily
    rotate %d
    maxage %d
    missingok
    notifempty
    compress
    delaycompress
    dateext
    dateformat -%%Y%%m%%d
    sharedscripts
    create 0640 www-data adm
    postrotate
        if [ -s /run/nginx.pid ]; then kill -USR1 "$(cat /run/nginx.pid)"; fi
        if [ -s /run/apache2/apache2.pid ]; then invoke-rc.d apache2 reload >/dev/null 2>&1 || true; fi
    endscript
}
`, days, nginxLogDir, apacheLogDir, days, days)
	if err := os.MkdirAll("/etc/logrotate.d", 0755); err != nil {
		return err
	}
	if err := os.WriteFile(logrotatePath, []byte(body), 0644); err != nil {
		return err
	}
	return nil
}

func Rotate() error {
	if err := WriteLogrotate(Retention()); err != nil {
		return err
	}
	ensureLogrotatePkg()
	bin, err := exec.LookPath("logrotate")
	if err != nil {
		return fmt.Errorf("logrotate is not installed")
	}
	cmd := exec.Command(bin, logrotatePath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("logrotate: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func ensureLogrotatePkg() {
	if _, err := exec.LookPath("logrotate"); err == nil {
		return
	}
	cmd := exec.Command("apt-get", "-y", "install", "logrotate")
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	_ = cmd.Run()
}

func EnsureSiteLogs() error {
	if err := EnsureLogDirs(); err != nil {
		return err
	}
	changed := false
	ngx, _ := filepath.Glob("/etc/nginx/sites-available/*.conf")
	for _, path := range ngx {
		domain := strings.TrimSuffix(filepath.Base(path), ".conf")
		if domain == "siroc-default" || domain == "default" {
			continue
		}
		if validate.Domain(domain) != nil {
			continue
		}
		_ = TouchSiteLogs(domain)
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		next, ok := injectNginxLogs(string(b), domain)
		if !ok {
			continue
		}
		if err := os.WriteFile(path, []byte(next), 0644); err != nil {
			return err
		}
		changed = true
	}
	ap, _ := filepath.Glob("/etc/apache2/sites-available/*.conf")
	for _, path := range ap {
		domain := strings.TrimSuffix(filepath.Base(path), ".conf")
		if domain == "siroc-backend" || domain == "000-default" || domain == "default" {
			continue
		}
		if validate.Domain(domain) != nil {
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		next, ok := injectApacheLogs(string(b), domain)
		if !ok {
			continue
		}
		if err := os.WriteFile(path, []byte(next), 0644); err != nil {
			return err
		}
		changed = true
	}
	if changed {
		_ = reloadNginx()
		if exec.Command("systemctl", "is-active", "--quiet", "apache2").Run() == nil {
			_ = exec.Command("systemctl", "reload", "apache2").Run()
		}
	}
	return nil
}

func injectNginxLogs(content, domain string) (string, bool) {
	marker := "/var/log/nginx/sites/" + domain
	if strings.Contains(content, marker) {
		return content, false
	}
	line := fmt.Sprintf("    access_log %s combined;\n    error_log %s;\n", NginxAccessLog(domain), NginxErrorLog(domain))
	anchor := "client_max_body_size"
	if !strings.Contains(content, anchor) {
		anchor = "server_name"
	}
	var b strings.Builder
	changed := false
	for _, raw := range strings.SplitAfter(content, "\n") {
		b.WriteString(raw)
		if strings.Contains(raw, anchor) {
			b.WriteString(line)
			changed = true
		}
	}
	return b.String(), changed
}

func injectApacheLogs(content, domain string) (string, bool) {
	wantErr := "${APACHE_LOG_DIR}/sites/" + domain + "-error.log"
	if strings.Contains(content, wantErr) {
		return content, false
	}
	oldErr := "${APACHE_LOG_DIR}/" + domain + "-error.log"
	oldAcc := "${APACHE_LOG_DIR}/" + domain + "-access.log"
	next := strings.ReplaceAll(content, oldErr, wantErr)
	next = strings.ReplaceAll(next, oldAcc, "${APACHE_LOG_DIR}/sites/"+domain+"-access.log")
	if next == content {
		if strings.Contains(content, "ErrorLog") {
			return content, false
		}
		insert := fmt.Sprintf("    ErrorLog ${APACHE_LOG_DIR}/sites/%s-error.log\n    CustomLog ${APACHE_LOG_DIR}/sites/%s-access.log combined\n", domain, domain)
		next = strings.Replace(content, "</VirtualHost>", insert+"</VirtualHost>", 1)
	}
	return next, next != content
}

func SiteDomains() []string {
	files, _ := filepath.Glob("/etc/nginx/sites-available/*.conf")
	var out []string
	for _, f := range files {
		d := strings.TrimSuffix(filepath.Base(f), ".conf")
		if d == "siroc-default" || d == "default" {
			continue
		}
		if validate.Domain(d) != nil {
			continue
		}
		out = append(out, d)
	}
	return out
}

func ReportPath(domain string) string {
	return filepath.Join(goaccessDir, domain+".html")
}

func ReportInfo(domain string) (generated time.Time, size int64, ok bool) {
	st, err := os.Stat(ReportPath(domain))
	if err != nil {
		return time.Time{}, 0, false
	}
	return st.ModTime().UTC(), st.Size(), true
}

package software

import (
	"log"
	"os"
	"os/exec"
	"strings"
)

const apacheRemoteIPPath = "/etc/apache2/conf-available/siroc-remoteip.conf"

// apacheRemoteIPConf tells Apache to use the visitor address Nginx sends.
// Apache only listens on localhost, so the TCP peer is always 127.0.0.1 and
// the access log, error log, and ModSecurity otherwise record that address.
func apacheRemoteIPConf() string {
	return `# Siroc. Nginx on this machine sends the visitor address in X-Real-IP.
<IfModule remoteip_module>
    RemoteIPHeader X-Real-IP
    RemoteIPInternalProxy 127.0.0.1
    RemoteIPInternalProxy ::1
</IfModule>
`
}

// EnsureApacheRemoteIP enables mod_remoteip so Apache and ModSecurity log the
// visitor address instead of 127.0.0.1.
func EnsureApacheRemoteIP() {
	if _, err := os.Stat("/etc/apache2/apache2.conf"); err != nil {
		return
	}
	if _, err := exec.LookPath("a2enmod"); err != nil {
		return
	}
	changed := false
	if out, err := exec.Command("a2enmod", "remoteip").CombinedOutput(); err != nil {
		log.Printf("apache real ip: %s", strings.TrimSpace(string(out)))
		return
	} else if strings.Contains(string(out), "Enabling module") {
		changed = true
	}
	body := apacheRemoteIPConf()
	if b, err := os.ReadFile(apacheRemoteIPPath); err != nil || string(b) != body {
		if err := os.MkdirAll("/etc/apache2/conf-available", 0755); err != nil {
			log.Printf("apache real ip: %v", err)
			return
		}
		if err := os.WriteFile(apacheRemoteIPPath, []byte(body), 0644); err != nil {
			log.Printf("apache real ip: %v", err)
			return
		}
		changed = true
	}
	if _, err := os.Lstat("/etc/apache2/conf-enabled/siroc-remoteip.conf"); err != nil {
		if out, err := exec.Command("a2enconf", "siroc-remoteip").CombinedOutput(); err != nil {
			log.Printf("apache real ip: %s", strings.TrimSpace(string(out)))
			return
		}
		changed = true
	}
	if !changed {
		return
	}
	if out, err := exec.Command("apache2ctl", "configtest").CombinedOutput(); err != nil {
		log.Printf("apache real ip: %s", strings.TrimSpace(string(out)))
		return
	}
	if exec.Command("systemctl", "is-active", "--quiet", "apache2").Run() == nil {
		if out, err := exec.Command("systemctl", "reload", "apache2").CombinedOutput(); err != nil {
			log.Printf("apache real ip: %s", strings.TrimSpace(string(out)))
		}
	}
}

//go:build linux

package software

import "os"

const sslRenewCronPath = "/etc/cron.d/siroc-ssl"

// InstallSSLRenew writes the twice-daily certbot renew cron and the deploy hook
// that reloads Nginx and Apache after a certificate is replaced.
func InstallSSLRenew() error {
	if err := os.MkdirAll("/var/www/letsencrypt/.well-known/acme-challenge", 0755); err != nil {
		return err
	}
	if err := os.MkdirAll("/etc/letsencrypt/renewal-hooks/deploy", 0755); err != nil {
		return err
	}
	if err := os.MkdirAll("/var/log/siroc", 0750); err != nil {
		return err
	}
	if err := os.WriteFile("/etc/letsencrypt/renewal-hooks/deploy/cp-reload-nginx.sh", []byte(SSLRenewHook()), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll("/etc/cron.d", 0755); err != nil {
		return err
	}
	tmp := sslRenewCronPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(SSLRenewCron()), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, sslRenewCronPath)
}

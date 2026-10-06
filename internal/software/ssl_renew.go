package software

// SSLRenewCron is /etc/cron.d/siroc-ssl. Certbot renews only certificates
// that are near expiry, and does nothing when certbot is not installed.
func SSLRenewCron() string {
	return `# Siroc Let's Encrypt renewal. Written by the panel.
SHELL=/bin/sh
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
17 3,15 * * * root sh -c 'command -v certbot >/dev/null 2>&1 && certbot renew --quiet --no-random-sleep-on-renew' >> /var/log/siroc/ssl-renew.log 2>&1
`
}

// SSLRenewHook runs after certbot deploys a renewed certificate.
func SSLRenewHook() string {
	return "#!/bin/sh\nsystemctl reload nginx >/dev/null 2>&1 || true\nsystemctl reload apache2 >/dev/null 2>&1 || true\n"
}

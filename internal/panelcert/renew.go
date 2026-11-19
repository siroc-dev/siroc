package panelcert

import "fmt"

// RenewService is the systemd unit that renews the panel certificate.
func RenewService(dataDir, exe string) string {
	return fmt.Sprintf(`[Unit]
Description=Renew the Siroc panel certificate
After=network-online.target

[Service]
Type=oneshot
Environment=SIROC_DATA_DIR=%s
ExecStart=%s issue-cert
`, dataDir, exe)
}

// RenewTimer runs the renewal once a day. issue-cert does nothing while the
// certificate still has more than two days left.
func RenewTimer() string {
	return `[Unit]
Description=Renew the Siroc panel certificate daily

[Timer]
OnCalendar=daily
Persistent=true
RandomizedDelaySec=1h

[Install]
WantedBy=timers.target
`
}

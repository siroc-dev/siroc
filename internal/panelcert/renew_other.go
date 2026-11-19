//go:build !linux

package panelcert

// InstallRenewTimer is a no-op outside Linux.
func InstallRenewTimer(string) error { return nil }

package software

import (
	"strings"
	"testing"
)

func TestSSLRenewCron(t *testing.T) {
	body := SSLRenewCron()
	if !strings.Contains(body, "17 3,15 * * * root ") {
		t.Fatalf("cron spec missing: %s", body)
	}
	if !strings.Contains(body, "certbot renew --quiet --no-random-sleep-on-renew") {
		t.Fatalf("renew command missing: %s", body)
	}
	if !strings.HasSuffix(body, "\n") {
		t.Fatal("cron file must end with a newline")
	}
	hook := SSLRenewHook()
	if !strings.Contains(hook, "reload nginx") || !strings.Contains(hook, "reload apache2") {
		t.Fatalf("hook = %q", hook)
	}
}

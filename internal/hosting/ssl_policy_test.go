package hosting

import "testing"

func TestCertPreferOrderUsesLocal(t *testing.T) {
	got := certPreferOrder("")
	if len(got) != 3 || got[0] != "local" || got[1] != "custom" || got[2] != "letsencrypt" {
		t.Fatal(got)
	}
	if certPreferOrder("local")[0] != "local" {
		t.Fatal(certPreferOrder("local"))
	}
	if certPreferOrder("letsencrypt")[0] != "letsencrypt" {
		t.Fatal(certPreferOrder("letsencrypt"))
	}
}

func TestSSLRedirectStaysOff(t *testing.T) {
	on := true
	off := false
	if sslRedirect(true, "letsencrypt", nil) || sslRedirect(true, "custom", nil) || sslRedirect(true, "local", nil) {
		t.Fatal("redirect turned on by itself")
	}
	if !sslRedirect(true, "local", &on) {
		t.Fatal("saved switch should redirect")
	}
	if sslRedirect(true, "letsencrypt", &off) || sslRedirect(false, "local", &on) {
		t.Fatal("explicit off or no ssl")
	}
}

package security

import "testing"

func TestNormalizeIgnore(t *testing.T) {
	got, err := NormalizeIgnore([]string{
		" 203.0.113.10 ",
		"203.0.113.10/32",
		"# comment",
		"203.0.113.5/24",
		"2001:db8::1",
		"",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2001:db8::1", "203.0.113.0/24", "203.0.113.10"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	for _, bad := range []string{"localhost", "127.0.0.1", "::1", "0.0.0.0/0", "::/0", "10.0.0.0/4", "1.2.3.4;touch", "not an ip"} {
		if _, err := NormalizeIgnore([]string{bad}); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestRenderFail2banIgnore(t *testing.T) {
	got, err := RenderFail2banIgnore([]string{"198.51.100.8", "203.0.113.0/24"})
	if err != nil {
		t.Fatal(err)
	}
	want := "# Siroc Fail2ban whitelist. Localhost is always ignored.\n[DEFAULT]\nignoreip = 127.0.0.1/8 ::1 198.51.100.8 203.0.113.0/24\n"
	if got != want {
		t.Fatalf("got %q", got)
	}
	if _, err := RenderFail2banIgnore(nil); err != nil {
		t.Fatal(err)
	}
}

func TestIPIgnored(t *testing.T) {
	addrs := []string{"203.0.113.10", "198.51.100.0/24"}
	if !IPIgnored("203.0.113.10", addrs) || !IPIgnored("198.51.100.20", addrs) || !IPIgnored("127.0.0.1", nil) {
		t.Fatal("expected match")
	}
	if IPIgnored("203.0.113.11", addrs) || IPIgnored("nope", addrs) {
		t.Fatal("unexpected match")
	}
}

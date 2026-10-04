package siteopts

import "testing"

func TestNormalizeRedirectsAndAccess(t *testing.T) {
	got, err := Normalize(Options{
		Index:  []string{"index.php", "index.php", "index.html"},
		Access: "allow",
		IPs:    []string{"10.0.0.0/8", "203.0.113.5"},
		Redirects: []Redirect{
			{From: "/old", To: "/new", Code: 0},
			{From: "/go", To: "https://example.com/x", Code: 302},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Index) != 2 || got.Index[0] != "index.php" {
		t.Fatalf("index %#v", got.Index)
	}
	if got.Redirects[0].Code != 301 {
		t.Fatalf("code %d", got.Redirects[0].Code)
	}
	extra, err := NginxExtra(got, "")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(extra, "location = /old") || !contains(extra, "return 301 /new;") {
		t.Fatalf("extra:\n%s", extra)
	}
	access, err := NginxAccess(got)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(access, "allow 10.0.0.0/8;") || !contains(access, "deny all;") {
		t.Fatalf("access:\n%s", access)
	}
	idx, err := ApacheIndex(got)
	if err != nil || idx != "index.php index.html" {
		t.Fatalf("index %q %v", idx, err)
	}
}

func TestRejectUnsafe(t *testing.T) {
	if _, err := Normalize(Options{Redirects: []Redirect{{From: "/a;rm", To: "/b"}}}); err == nil {
		t.Fatal("expected bad path")
	}
	if _, err := Normalize(Options{Redirects: []Redirect{{From: "/.well-known/acme-challenge/x", To: "/b"}}}); err == nil {
		t.Fatal("expected well-known rejection")
	}
	if _, err := Normalize(Options{Access: "allow"}); err == nil {
		t.Fatal("expected empty allow list")
	}
	if _, err := Normalize(Options{IPs: []string{"not-an-ip"}, Access: "deny"}); err == nil {
		t.Fatal("expected bad ip")
	}
	if _, err := NginxExtra(Options{Hotlink: true}, "http://evil; rm"); err == nil {
		t.Fatal("expected bad proxy")
	}
}

func TestMaintenanceAndHotlink(t *testing.T) {
	extra, err := NginxExtra(Options{Maintenance: true, Hotlink: true}, "http://127.0.0.1:3000/")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(extra, "return 503;") || !contains(extra, "acme-challenge") {
		t.Fatalf("maintenance missing:\n%s", extra)
	}
	if !contains(extra, "valid_referers") || !contains(extra, "proxy_pass http://127.0.0.1:3000/;") {
		t.Fatalf("hotlink missing:\n%s", extra)
	}
}

func contains(s, part string) bool {
	return len(s) >= len(part) && (s == part || len(part) == 0 || (len(s) > 0 && indexOf(s, part) >= 0))
}

func indexOf(s, part string) int {
	for i := 0; i+len(part) <= len(s); i++ {
		if s[i:i+len(part)] == part {
			return i
		}
	}
	return -1
}

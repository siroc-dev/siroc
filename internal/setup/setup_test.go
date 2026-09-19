package setup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureValidClear(t *testing.T) {
	dir := t.TempDir()
	tok, err := Ensure(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) != 64 {
		t.Fatalf("token length %d", len(tok))
	}
	again, err := Ensure(dir)
	if err != nil || again != tok {
		t.Fatalf("Ensure should reuse token: %v %q %q", err, again, tok)
	}
	if !Valid(dir, tok) {
		t.Fatal("valid token rejected")
	}
	if Valid(dir, "nope") || Valid(dir, tok+"x") || Valid(dir, "") {
		t.Fatal("invalid token accepted")
	}
	if err := WriteURL(dir, "https://example.test/setup?token="+tok); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, URLFile)); err != nil {
		t.Fatal(err)
	}
	Clear(dir)
	if Valid(dir, tok) {
		t.Fatal("token still valid after Clear")
	}
}

func TestPublicURL(t *testing.T) {
	t.Setenv("SIROC_PUBLIC_HOST", "203.0.113.10")
	got := PublicURL(":8443", true, "abc")
	want := "https://203.0.113.10:8443/setup?token=abc"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	got = PublicURL(":80", false, "abc")
	want = "http://203.0.113.10/setup?token=abc"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

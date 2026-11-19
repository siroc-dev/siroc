package setup

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormatLinks(t *testing.T) {
	got := FormatLinks(":8443", true, "abc", "10.0.0.8", "203.0.113.10")
	want := "Private: https://10.0.0.8:8443/setup?token=abc\nPublic:  https://203.0.113.10:8443/setup?token=abc"
	if got != want {
		t.Fatalf("got %q", got)
	}
	got = FormatLinks(":8443", true, "abc", "203.0.113.10", "203.0.113.10")
	if got != "https://203.0.113.10:8443/setup?token=abc" {
		t.Fatalf("same host got %q", got)
	}
	got = FormatLinks(":443", true, "abc", "10.0.0.8", "")
	if !strings.Contains(got, "Private: https://10.0.0.8/setup?token=abc") || !strings.Contains(got, PublicIPUnavailable) {
		t.Fatalf("missing public note: %q", got)
	}
	got = FormatLinks(":80", false, "", "10.0.0.8", "203.0.113.10")
	want = "Private: http://10.0.0.8\nPublic:  http://203.0.113.10"
	if got != want {
		t.Fatalf("panel links got %q", got)
	}
}

func TestLookupPublicIPv4(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "203.0.113.10\n")
	}))
	defer srv.Close()
	got, err := LookupPublicIPv4(srv.Client(), srv.URL)
	if err != nil || got != "203.0.113.10" {
		t.Fatalf("got %q err %v", got, err)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "not-an-ip")
	}))
	defer bad.Close()
	if _, err := LookupPublicIPv4(bad.Client(), bad.URL); err == nil {
		t.Fatal("expected invalid address to fail")
	}
}

func TestSaveLinksSkipsLookupWhenHostSet(t *testing.T) {
	t.Setenv("SIROC_PUBLIC_HOST", "203.0.113.10")
	dir := t.TempDir()
	text, err := SaveLinks(dir, ":8443", true, "abc")
	if err != nil {
		t.Fatal(err)
	}
	want := FormatLinks(":8443", true, "abc", PrivateHost(), "203.0.113.10")
	if text != want {
		t.Fatalf("got %q want %q", text, want)
	}
	body, err := os.ReadFile(filepath.Join(dir, URLFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimRight(string(body), "\n") != text {
		t.Fatalf("file %q", body)
	}
}

package hosting

import (
	"strings"
	"testing"
)

func TestSetListenDefault(t *testing.T) {
	src := "server {\n    listen 80;\n    listen [::]:80;\n    listen 443 ssl;\n    listen 127.0.0.1:8080 default_server;\n    # listen 80;\n}\n"
	on := SetListenDefault(src, true)
	for _, want := range []string{
		"    listen 80 default_server;",
		"    listen [::]:80 default_server;",
		"    listen 443 ssl default_server;",
		"    listen 127.0.0.1:8080 default_server;",
		"    # listen 80;",
	} {
		if !strings.Contains(on, want) {
			t.Fatalf("missing %s\n%s", want, on)
		}
	}
	if SetListenDefault(on, true) != on {
		t.Fatal("adding default_server twice changed the file")
	}
	off := SetListenDefault(on, false)
	if strings.Contains(off, "listen 80 default_server") || strings.Contains(off, "listen 443 ssl default_server") {
		t.Fatalf("still default:\n%s", off)
	}
	if !strings.Contains(off, "    listen 127.0.0.1:8080 default_server;") {
		t.Fatalf("loopback changed:\n%s", off)
	}
	if !strings.Contains(off, "    listen 443 ssl;") || !strings.Contains(off, "    listen 80;") {
		t.Fatalf("listen flags dropped:\n%s", off)
	}
	http2 := SetListenDefault("    listen 443 ssl http2;", true)
	if http2 != "    listen 443 ssl http2 default_server;" {
		t.Fatalf("%s", http2)
	}
	unix := SetListenDefault("    listen unix:/run/nginx.sock;", true)
	if unix != "    listen unix:/run/nginx.sock;" {
		t.Fatalf("%s", unix)
	}
	twice := SetListenDefault("    listen 80;\n    listen 80;\n", true)
	if twice != "    listen 80 default_server;\n    listen 80;\n" {
		t.Fatalf("%s", twice)
	}
}

func TestCleanDefaultDomain(t *testing.T) {
	got, err := cleanDefaultDomain(" Example.COM ")
	if err != nil || got != "example.com" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := cleanDefaultDomain("../etc/passwd"); err == nil {
		t.Fatal("path should be rejected")
	}
	if got, err := cleanDefaultDomain("  "); err != nil || got != "" {
		t.Fatalf("empty %q %v", got, err)
	}
}

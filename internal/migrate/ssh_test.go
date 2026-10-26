package migrate

import (
	"strings"
	"testing"
)

func TestParseSSHHost(t *testing.T) {
	host, port, err := ParseSSHHost("203.0.113.10", 0)
	if err != nil || host != "203.0.113.10" || port != 22 {
		t.Fatalf("default: %s %d %v", host, port, err)
	}
	host, port, err = ParseSSHHost("ssh://root@203.0.113.10:2200/home", 0)
	if err != nil || host != "203.0.113.10" || port != 2200 {
		t.Fatalf("url: %s %d %v", host, port, err)
	}
	host, port, err = ParseSSHHost("[2001:db8::1]:22", 0)
	if err != nil || host != "2001:db8::1" || port != 22 {
		t.Fatalf("ipv6: %s %d %v", host, port, err)
	}
	if _, _, err := ParseSSHHost("", 22); err == nil {
		t.Fatal("empty host")
	}
}

func TestParseSSHSites(t *testing.T) {
	sites, err := ParseSSHSites("alice\tshop.test\nalice\tshop.test\nbob blog.example\nnope\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 2 || sites[0].Domain != "blog.example" || sites[0].Owner != "bob" || sites[1].Owner != "alice" {
		t.Fatalf("%#v", sites)
	}
	if _, err := ParseSSHSites("not a domain\n"); err == nil {
		t.Fatal("expected no websites")
	}
}

func TestParseReady(t *testing.T) {
	n, path, err := parseReady("ready 4096 /mnt/data/siroc-tmp/siroc-migrate-shop.example-20261026-000000.tar.gz\n")
	if err != nil || n != 4096 || !strings.Contains(path, "/mnt/data/siroc-tmp/") {
		t.Fatalf("%d %s %v", n, path, err)
	}
	if _, _, err := parseReady("ready 4096\n"); err == nil {
		t.Fatal("missing path")
	}
	if _, _, err := parseReady("ready 0 /siroc-tmp/siroc-migrate-a.tar.gz\n"); err == nil {
		t.Fatal("empty archive")
	}
	if _, _, err := parseReady("ready 10 /tmp/siroc-migrate-a.tar.gz\n"); err == nil {
		t.Fatal("path outside siroc-tmp")
	}
	if _, _, err := parseReady("ready 10 /mnt/data/siroc-tmp/../etc/siroc-migrate-a.tar.gz\n"); err == nil {
		t.Fatal("path escape")
	}
}

func TestShellQuote(t *testing.T) {
	if shellQuote("a'b") != `'a'\''b'` {
		t.Fatal(shellQuote("a'b"))
	}
}

func TestCopyScriptIncludesDatabasesAndFTP(t *testing.T) {
	for _, needle := range []string{
		"ftp.passwd",
		"mysql.conf",
		"mysql.list",
		"SHOW DATABASES",
		"could not dump database",
		"unknown database",
	} {
		if !strings.Contains(copySiteScript, needle) {
			t.Fatalf("copy script missing %s", needle)
		}
	}
}

package migrate

import "testing"

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

func TestShellQuote(t *testing.T) {
	if shellQuote("a'b") != `'a'\''b'` {
		t.Fatal(shellQuote("a'b"))
	}
}

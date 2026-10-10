package validate

import (
	"fmt"
	"strings"
	"testing"
)

func TestDomainAliasesWildcard(t *testing.T) {
	got, err := DomainAliases("strawbandco.shop", []string{"*.strawbandco.shop", "www.strawbandco.shop", " *.strawbandco.shop "})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "*.strawbandco.shop" || got[1] != "www.strawbandco.shop" {
		t.Fatalf("got %#v", got)
	}
	if _, err := DomainAliases("strawbandco.shop", []string{"*"}); err == nil {
		t.Fatal("bare * should fail")
	}
	if _, err := DomainAliases("strawbandco.shop", []string{"*.shop"}); err == nil {
		t.Fatal("*.shop should fail")
	}
	if err := Domain("*.strawbandco.shop"); err == nil {
		t.Fatal("primary domain cannot be a wildcard")
	}
}

func TestIDNDomain(t *testing.T) {
	ascii, err := ASCIIHost("münchen.de")
	if err != nil || ascii != "xn--mnchen-3ya.de" {
		t.Fatalf("ascii %q %v", ascii, err)
	}
	if DisplayDomain(ascii) != "münchen.de" {
		t.Fatalf("display %q", DisplayDomain(ascii))
	}
	if DisplayDomain("example.com") != "example.com" {
		t.Fatal("ascii display changed")
	}
	got, err := DomainAliases("münchen.de", []string{"*.münchen.de", "www.münchen.de"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "*.xn--mnchen-3ya.de" || got[1] != "www.xn--mnchen-3ya.de" {
		t.Fatalf("aliases %#v", got)
	}
	if DisplayDomain(got[0]) != "*.münchen.de" {
		t.Fatalf("wild display %q", DisplayDomain(got[0]))
	}
}

func TestFTPHome(t *testing.T) {
	got, err := FTPHome("/home", "alice", "")
	if err != nil || got != "/home/alice" {
		t.Fatalf("default %s %v", got, err)
	}
	got, err = FTPHome("/home", "alice", "domains/a.test/public_html")
	if err != nil || got != "/home/alice/domains/a.test/public_html" {
		t.Fatalf("rel %s %v", got, err)
	}
	got, err = FTPHome("/home", "alice", "/mnt/data/alice")
	if err != nil || got != "/mnt/data/alice" {
		t.Fatalf("disk %s %v", got, err)
	}
	if _, err := FTPHome("/home", "alice", "../bob"); err == nil {
		t.Fatal("escape should fail")
	}
	if _, err := FTPHome("/home", "alice", "/etc/nginx"); err == nil {
		t.Fatal("system path should fail")
	}
}

func TestDomainAliasLimit(t *testing.T) {
	names := make([]string, MaxDomainAliases)
	for i := range names {
		names[i] = fmt.Sprintf("a%d.example.com", i)
	}
	got, err := DomainAliases("example.com", names)
	if err != nil || len(got) != MaxDomainAliases {
		t.Fatalf("%v len %d", err, len(got))
	}
	names = append(names, "extra.example.com")
	if _, err := DomainAliases("example.com", names); err == nil {
		t.Fatal("over the limit")
	}
}

func TestCertbotNames(t *testing.T) {
	aliases := []string{"*.example.com", "www.example.com"}
	for i := 0; i < 120; i++ {
		aliases = append(aliases, fmt.Sprintf("n%d.example.com", i))
	}
	got := CertbotNames("example.com", aliases)
	if len(got) != MaxCertbotNames || got[0] != "example.com" || got[1] != "www.example.com" {
		t.Fatalf("%d names, first %#v", len(got), got[:2])
	}
	for _, n := range got {
		if strings.HasPrefix(n, "*.") {
			t.Fatalf("wildcard included: %s", n)
		}
	}
}

package validate

import "testing"

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

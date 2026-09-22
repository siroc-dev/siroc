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
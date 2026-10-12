package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAliasTimesKeepExistingNames(t *testing.T) {
	names, added := parseAliasStore(`["www.example.com"]`)
	if len(names) != 1 || names[0] != "www.example.com" || added != nil {
		t.Fatalf("old list %#v %#v", names, added)
	}
	raw, err := encodeAliases(`["www.example.com"]`, []string{"www.example.com", "blog.example.com"}, time.Date(2026, 10, 13, 4, 10, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	names, added = parseAliasStore(raw)
	if len(names) != 2 || names[1] != "blog.example.com" || added[0] != "" || added[1] != "2026-10-13 04:10:00" {
		t.Fatalf("merged %#v %#v", names, added)
	}
	again, err := encodeAliases(raw, []string{"blog.example.com"}, time.Date(2026, 10, 14, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	names, added = parseAliasStore(again)
	if len(names) != 1 || names[0] != "blog.example.com" || added[0] != "2026-10-13 04:10:00" {
		t.Fatalf("kept %#v %#v", names, added)
	}
}

func TestCreateSiteStoresAliasTime(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	acc, err := st.CreateAccount("a01", 1001, 1001, "", true, true)
	if err != nil {
		t.Fatal(err)
	}
	site, err := st.CreateSite(acc.ID, "a01.test", "/home/a01/domains/a01.test/public_html", "8.4", []string{"www.a01.test"}, "php", "", "", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(site.Aliases) != 1 || site.Aliases[0] != "www.a01.test" || site.AliasAdded[0] == "" {
		t.Fatalf("created %#v %#v", site.Aliases, site.AliasAdded)
	}
	first := site.AliasAdded[0]
	if err := st.UpdateSite(site.ID, site.PHPVersion, site.DocRoot, true, []string{"www.a01.test", "cdn.a01.test"}, false, "", "", ""); err != nil {
		t.Fatal(err)
	}
	site, err = st.GetSite(site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if site.AliasAdded[0] != first || site.Aliases[1] != "cdn.a01.test" || site.AliasAdded[1] == "" {
		t.Fatalf("updated %#v %#v", site.Aliases, site.AliasAdded)
	}
}

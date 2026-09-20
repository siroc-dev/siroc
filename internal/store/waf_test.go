package store

import (
	"path/filepath"
	"testing"
)

func TestWAFDefaultsAndToggle(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	acc, err := st.CreateAccount("a01", 1001, 1001, "", true, true)
	if err != nil {
		t.Fatal(err)
	}
	if !acc.WAFEnabled {
		t.Fatal("new accounts should have WAF on")
	}
	site, err := st.CreateSite(acc.ID, "a01.test", "/home/a01/domains/a01.test/public_html", "8.4", nil, "php", "", "", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if !site.WAFEnabled {
		t.Fatal("new sites should have WAF on")
	}
	if !EffectiveWAF(acc.WAFEnabled, site.WAFEnabled) {
		t.Fatal("expected effective WAF on")
	}
	if err := st.UpdateAccountWAF(acc.Username, false); err != nil {
		t.Fatal(err)
	}
	acc, err = st.GetAccount(acc.ID)
	if err != nil || acc.WAFEnabled {
		t.Fatalf("account waf: %+v %v", acc, err)
	}
	if EffectiveWAF(acc.WAFEnabled, site.WAFEnabled) {
		t.Fatal("account off should disable effective WAF")
	}
	if err := st.UpdateSiteWAF(site.ID, false); err != nil {
		t.Fatal(err)
	}
	site, err = st.GetSite(site.ID)
	if err != nil || site.WAFEnabled {
		t.Fatalf("site waf: %+v %v", site, err)
	}
	list, err := st.ListSites()
	if err != nil || len(list) != 1 || list[0].WAFEnabled {
		t.Fatalf("list: %+v %v", list, err)
	}
}

func TestEffectiveWAF(t *testing.T) {
	if EffectiveWAF(false, true) || EffectiveWAF(true, false) || EffectiveWAF(false, false) {
		t.Fatal("expected off")
	}
	if !EffectiveWAF(true, true) {
		t.Fatal("expected on")
	}
}

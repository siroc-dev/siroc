package store

import (
	"path/filepath"
	"strings"
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
	if err := st.UpdateSiteWAFRules(site.ID, []int{942100, 941100, 942100}); err != nil {
		t.Fatal(err)
	}
	site, err = st.GetSite(site.ID)
	if err != nil || len(site.WAFDisabledIDs) != 2 || site.WAFDisabledIDs[0] != 941100 || site.WAFDisabledIDs[1] != 942100 {
		t.Fatalf("rules: %+v %v", site.WAFDisabledIDs, err)
	}
	if err := st.UpdateSiteWAFRules(site.ID, []int{1}); err == nil {
		t.Fatal("expected invalid rule id")
	}
	if err := st.UpdateSiteWAFPaths(site.ID, []string{"/wp-admin", "/api/upload", "/wp-admin"}); err != nil {
		t.Fatal(err)
	}
	site, err = st.GetSite(site.ID)
	if err != nil || len(site.WAFPaths) != 2 || site.WAFPaths[0] != "/api/upload" || site.WAFPaths[1] != "/wp-admin" {
		t.Fatalf("paths: %+v %v", site.WAFPaths, err)
	}
	if len(site.WAFDisabledIDs) != 2 {
		t.Fatal("saving paths must keep disabled rules")
	}
}

func TestNormalizeWAFPaths(t *testing.T) {
	got, err := NormalizeWAFPaths([]string{" /wp-admin ", "/file.php"})
	if err != nil || len(got) != 2 || got[1] != "/wp-admin" {
		t.Fatalf("normalize: %+v %v", got, err)
	}
	rules := WAFPathRules([]string{"/wp-admin", "/file.php"})
	if !strings.Contains(rules, `^/wp\-admin(?:/|$)`) || !strings.Contains(rules, `^/file\.php(?:/|$)`) || !strings.Contains(rules, "ctl:ruleEngine=Off") || !strings.Contains(rules, "id:2100001") {
		t.Fatalf("rules:\n%s", rules)
	}
	for _, bad := range []string{"/", "/../etc", "/a?x=1", "/a b", "wp-admin", "/ok<script>"} {
		if _, err := NormalizeWAFPaths([]string{bad}); err == nil {
			t.Fatalf("accepted %q", bad)
		}
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

package security

import "testing"

func TestDefaultWAFPacksSkipScanner(t *testing.T) {
	off := map[string]bool{}
	for _, id := range defaultOffWAFPacks() {
		off[id] = true
	}
	for _, id := range []string{"scanner", "session", "reputation", "dos", "leakage"} {
		if !off[id] {
			t.Fatalf("want %s off by default", id)
		}
	}
	for _, id := range defaultWAFPacks() {
		if off[id] {
			t.Fatalf("%s should not be a default pack", id)
		}
	}
	got := mergeWAFRuleIDs([]int{941100, 920270}, defaultSEOWAFRules())
	if got[0] != 913100 || got[len(got)-1] != 941100 {
		t.Fatalf("%v", got)
	}
	foundWin := false
	for _, id := range got {
		if id == 932110 || id == 932120 {
			foundWin = true
		}
	}
	if !foundWin {
		t.Fatal("expected Windows CMD/PowerShell rule ids")
	}
	if len(dropWAFPacks([]string{"scanner", "session", "xss"}, defaultOffWAFPacks()...)) != 1 {
		t.Fatal("drop")
	}
}

func TestApplyWAFStateDefaultsTurnsOffExistingPacks(t *testing.T) {
	st := applyWAFStateDefaults(wafState{
		Mode:       "On",
		Packs:      []string{"scanner", "session", "xss", "sqli", "rce", "leakage"},
		SEORelaxed: 3,
	})
	for _, id := range st.Packs {
		if id == "scanner" || id == "session" || id == "reputation" || id == "dos" || id == "leakage" {
			t.Fatalf("pack %s still on", id)
		}
	}
	if st.SEORelaxed != wafDefaultsVersion {
		t.Fatalf("seoRelaxed %d", st.SEORelaxed)
	}
	found := false
	for _, id := range st.DisabledIDs {
		if id == 932110 {
			found = true
		}
	}
	if !found {
		t.Fatal("windows rule ids not merged")
	}
}

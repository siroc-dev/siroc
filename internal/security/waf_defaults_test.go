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
	got := mergeWAFRuleIDs([]int{942100, 920270}, defaultSEOWAFRules())
	if got[0] != 913100 || got[len(got)-1] != 942100 {
		t.Fatalf("%v", got)
	}
	has941100, has941160 := false, false
	for _, id := range got {
		if id == 941100 {
			has941100 = true
		}
		if id == 941160 {
			has941160 = true
		}
	}
	if !has941100 || !has941160 {
		t.Fatal("expected XSS rule ids 941100 and 941160")
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
		SEORelaxed: 4,
	})
	for _, id := range st.Packs {
		if id == "scanner" || id == "session" || id == "reputation" || id == "dos" || id == "leakage" {
			t.Fatalf("pack %s still on", id)
		}
	}
	if st.SEORelaxed != wafDefaultsVersion {
		t.Fatalf("seoRelaxed %d", st.SEORelaxed)
	}
	foundWin, has941100, has941160 := false, false, false
	for _, id := range st.DisabledIDs {
		if id == 932110 {
			foundWin = true
		}
		if id == 941100 {
			has941100 = true
		}
		if id == 941160 {
			has941160 = true
		}
	}
	if !foundWin {
		t.Fatal("windows rule ids not merged")
	}
	has920450 := false
	for _, id := range st.DisabledIDs {
		if id == 920450 {
			has920450 = true
		}
	}
	if !has920450 {
		t.Fatal("rule 920450 not merged")
	}
	if !has941100 || !has941160 {
		t.Fatal("xss rule ids not merged")
	}
}

package layout

import "testing"

func TestRewriteUnit(t *testing.T) {
	in := "" +
		"Environment=SIROC_ROOT=/opt/siroc\n" +
		"Environment=SIROC_WEB_DIR=/opt/siroc/web/dist\n" +
		"Environment=SIROC_DATA_DIR=/var/lib/siroc\n" +
		"ExecStart=/usr/local/bin/siroc-agent\n" +
		"ExecStart=/usr/local/bin/siroc-panel\n"
	got, changed := RewriteUnit(in)
	if !changed {
		t.Fatal("expected a change")
	}
	for _, want := range []string{
		"Environment=SIROC_ROOT=/siroc\n",
		"Environment=SIROC_WEB_DIR=/siroc/web/dist\n",
		"Environment=SIROC_DATA_DIR=/siroc/data\n",
		"ExecStart=/siroc/bin/siroc-agent\n",
		"ExecStart=/siroc/bin/siroc-panel\n",
	} {
		if !contains(got, want) {
			t.Fatalf("missing %q\n%s", want, got)
		}
	}
	again, changed := RewriteUnit(got)
	if changed || again != got {
		t.Fatal("second pass should be a no-op")
	}
}

func TestMissingAppArmorRules(t *testing.T) {
	old := "# Added by siroc so data under /siroc stays readable.\n/siroc/db/ r,\n/siroc/db/** rwk,\n"
	got := MissingAppArmorRules(old)
	if !contains(got, "/siroc/etc/ r,\n") || !contains(got, "/siroc/etc/** rw,\n") {
		t.Fatalf("etc rules missing:\n%s", got)
	}
	if contains(got, "/siroc/db/") {
		t.Fatalf("db rules should stay put:\n%s", got)
	}
	if MissingAppArmorRules(old+got) != "" {
		t.Fatal("complete profile should need nothing")
	}
}

func contains(s, part string) bool {
	return len(s) >= len(part) && (s == part || len(part) == 0 || indexOf(s, part) >= 0)
}

func indexOf(s, part string) int {
	for i := 0; i+len(part) <= len(s); i++ {
		if s[i:i+len(part)] == part {
			return i
		}
	}
	return -1
}

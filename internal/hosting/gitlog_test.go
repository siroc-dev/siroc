package hosting

import "testing"

func TestParseGitLog(t *testing.T) {
	in := "$ git log -1 --format=%H%n%s%n%an%n%aI\n0123456789abcdef0123456789abcdef01234567\nFix login timeout\nAda Lovelace\n2026-10-05T03:00:00+07:00\n"
	got, ok := ParseGitLog(in)
	if !ok {
		t.Fatal("expected commit")
	}
	if got.Hash != "0123456789ab" || got.Subject != "Fix login timeout" || got.Author != "Ada Lovelace" || got.At == "" {
		t.Fatalf("%+v", got)
	}
	if _, ok := ParseGitLog("not a commit\n"); ok {
		t.Fatal("accepted garbage")
	}
	short, ok := ParseGitLog("abc\nsubject\nauthor\n")
	if ok || short.Subject != "" {
		t.Fatal("accepted a short hash")
	}
}

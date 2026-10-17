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

func TestParseSymrefHead(t *testing.T) {
	got, ok := ParseSymrefHead("ref: refs/heads/develop\tHEAD\nabc\tHEAD\n")
	if !ok || got != "develop" {
		t.Fatalf("%q %v", got, ok)
	}
	if _, ok := ParseSymrefHead("abc\tHEAD\n"); ok {
		t.Fatal("missing symref")
	}
}

func TestParseRemoteBranches(t *testing.T) {
	in := "$ git ls-remote --symref git@github.com:org/repo.git\n" +
		"ref: refs/heads/main\tHEAD\n" +
		"aaa\tHEAD\n" +
		"ccc\trefs/heads/main\n" +
		"bbb\trefs/heads/develop\n" +
		"ddd\trefs/tags/v1.0\n"
	got, head := ParseRemoteBranches(in)
	if head != "main" || len(got) != 2 || got[0] != "main" || got[1] != "develop" {
		t.Fatalf("head %q branches %v", head, got)
	}
	if branches, head := ParseRemoteBranches("no refs\n"); head != "" || len(branches) != 0 {
		t.Fatalf("empty: %q %v", head, branches)
	}
}

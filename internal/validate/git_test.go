package validate

import (
	"strings"
	"testing"
)

func TestGitRepo(t *testing.T) {
	ok := []string{
		"git@github.com:org/repo.git",
		"git@gitlab.com:group/app.git",
		"ssh://git@github.com/org/repo.git",
	}
	for _, in := range ok {
		if _, err := GitRepo(in); err != nil {
			t.Fatalf("%s: %v", in, err)
		}
	}
	bad := []string{"", "https://github.com/org/repo.git", "git@github.com; rm -rf /", "git@host", "org/repo"}
	for _, in := range bad {
		if _, err := GitRepo(in); err == nil {
			t.Fatalf("%s should fail", in)
		}
	}
}

func TestGitCommand(t *testing.T) {
	if _, err := GitCommand(""); err != nil {
		t.Fatal(err)
	}
	if _, err := GitCommand(LaravelDeployCommand); err != nil {
		t.Fatal(err)
	}
	if _, err := GitCommand(NPMDeployCommand); err != nil {
		t.Fatal(err)
	}
	if _, err := GitCommand(strings.Repeat("x", 8193)); err == nil {
		t.Fatal("long command should fail")
	}
}

func TestGitBranch(t *testing.T) {
	n, err := GitBranch("")
	if err != nil || n != "main" {
		t.Fatalf("default: %q %v", n, err)
	}
	if _, err := GitBranch("release/1.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := GitBranch("../etc"); err == nil {
		t.Fatal(".. should fail")
	}
}

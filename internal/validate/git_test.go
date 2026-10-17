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
		"https://github.com/org/repo.git",
		"http://git.example.com/org/repo",
	}
	for _, in := range ok {
		if _, err := GitRepo(in); err != nil {
			t.Fatalf("%s: %v", in, err)
		}
	}
	bad := []string{"", "git@github.com; rm -rf /", "git@host", "org/repo", "https://github.com"}
	for _, in := range bad {
		if _, err := GitRepo(in); err == nil {
			t.Fatalf("%s should fail", in)
		}
	}
}

func TestParseGitRemoteBranch(t *testing.T) {
	repo, branch, err := ParseGitRemote("https://github.com/org/repo/tree/develop")
	if err != nil || repo != "https://github.com/org/repo.git" || branch != "develop" {
		t.Fatalf("%s %s %v", repo, branch, err)
	}
	repo, branch, err = ParseGitRemote("https://gitlab.com/group/app/-/tree/release/1.0")
	if err != nil || repo != "https://gitlab.com/group/app.git" || branch != "release/1.0" {
		t.Fatalf("%s %s %v", repo, branch, err)
	}
	repo, branch, err = ParseGitRemote("https://github.com/org/repo.git#main")
	if err != nil || repo != "https://github.com/org/repo.git" || branch != "main" {
		t.Fatalf("%s %s %v", repo, branch, err)
	}
	repo, branch, err = ParseGitRemote("git@github.com:org/repo.git")
	if err != nil || repo != "git@github.com:org/repo.git" || branch != "" {
		t.Fatalf("%s %s %v", repo, branch, err)
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

func TestGitNeedsDeployKey(t *testing.T) {
	if !GitNeedsDeployKey("git@github.com: Permission denied (publickey).") {
		t.Fatal("publickey")
	}
	if !GitNeedsDeployKey("ERROR: Repository not found.") {
		t.Fatal("private github")
	}
	if GitNeedsDeployKey("composer install failed") {
		t.Fatal("build failure is not a key problem")
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

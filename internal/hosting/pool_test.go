package hosting

import "testing"

func TestSetPoolDirectiveChdir(t *testing.T) {
	in := "[user-example.com]\nuser = user\nchdir = /\nchdir = /tmp\nlisten = /run/php/x.sock\n"
	got := setPoolDirective(in, "chdir", "/home/user")
	want := "[user-example.com]\nuser = user\nchdir = /home/user\nlisten = /run/php/x.sock\n"
	if got != want {
		t.Fatalf("got %q", got)
	}
	again := setPoolDirective(got, "chdir", "/home/user")
	if again != got {
		t.Fatal("rewriting the same chdir changed the file")
	}
	added := setPoolDirective("user = user\n", "chdir", "/home/user")
	if added != "user = user\nchdir = /home/user\n" {
		t.Fatalf("got %q", added)
	}
}

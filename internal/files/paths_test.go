package files

import (
	"strings"
	"testing"
)

func TestExternalDocRootIsNotJoinedOntoHome(t *testing.T) {
	home := "/home/abraventure"
	roots := []string{"/mnt/data/plyxxx"}
	got, err := candidatePath(home, "/mnt/data/plyxxx", roots)
	if err != nil {
		t.Fatal(err)
	}
	if got != "/mnt/data/plyxxx" {
		t.Fatalf("got %s", got)
	}
	if strings.Contains(got, "/home/abraventure/mnt") {
		t.Fatalf("joined onto home: %s", got)
	}
	sub, err := candidatePath(home, "/mnt/data/plyxxx/index.php", roots)
	if err != nil {
		t.Fatal(err)
	}
	if sub != "/mnt/data/plyxxx/index.php" {
		t.Fatalf("child got %s", sub)
	}
}

func TestHomeRelativePathStillJoinsHome(t *testing.T) {
	home := "/home/abraventure"
	got, err := candidatePath(home, "/domains/abraventure.com/public_html", []string{"/mnt/data/plyxxx"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "/home/abraventure/domains/abraventure.com/public_html" {
		t.Fatalf("got %s", got)
	}
	if _, err := candidatePath(home, "../etc/passwd", nil); err == nil {
		t.Fatal("expected escape to be rejected")
	}
}

func TestCleanSiteRoots(t *testing.T) {
	got := cleanSiteRoots("/home", "abraventure", []string{
		"/mnt/data/plyxxx",
		"/mnt/data/plyxxx",
		"/mnt",
		"/etc/nginx",
		"/home/other/www",
		"/home/abraventure/domains/abraventure.com/public_html",
	})
	if len(got) != 1 || got[0] != "/mnt/data/plyxxx" {
		t.Fatalf("got %#v", got)
	}
}

func TestDisplayPath(t *testing.T) {
	home := "/home/abraventure"
	if got := displayPath(home, "/home/abraventure/domains/a"); got != "/domains/a" {
		t.Fatalf("home path: %s", got)
	}
	if got := displayPath(home, "/mnt/data/plyxxx"); got != "/mnt/data/plyxxx" {
		t.Fatalf("external path: %s", got)
	}
	if !pathAllowed(home, "/mnt/data/plyxxx/a", []string{"/mnt/data/plyxxx"}) {
		t.Fatal("child of the document root should be allowed")
	}
	if pathAllowed(home, "/mnt/data", []string{"/mnt/data/plyxxx"}) {
		t.Fatal("parent of the document root should stay outside the jail")
	}
	if pathAllowed(home, "/etc/passwd", []string{"/mnt/data/plyxxx"}) {
		t.Fatal("system path should stay outside the jail")
	}
}

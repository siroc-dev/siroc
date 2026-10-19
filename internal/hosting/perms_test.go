package hosting

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWithinHome(t *testing.T) {
	home := filepath.Join(string(filepath.Separator), "home", "mangablackcat")
	inside := filepath.Join(home, "domains", "mangablackcat.com", "public_html", "storage", "app", "public")
	if !withinHome(home, inside) {
		t.Fatal("storage target should stay inside the home")
	}
	if withinHome(home, filepath.Join(string(filepath.Separator), "etc", "ssh")) {
		t.Fatal("system path must stay outside")
	}
	if docInAccount(home, filepath.Join(string(filepath.Separator), "mnt", "data", "plyxxx")) {
		t.Fatal("a mounted document root must not be treated as the account tree")
	}
	if !docInAccount(home, filepath.Join(home, "domains", "abraventure.com", "public_html")) {
		t.Fatal("a site inside the home is the account tree")
	}
}

func TestChmodWebTreeFollowsStorageLink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory modes are not unix modes on windows")
	}
	home := t.TempDir()
	site := filepath.Join(home, "domains", "mangablackcat.com", "public_html")
	target := filepath.Join(site, "storage", "app", "public")
	linkParent := filepath.Join(site, "public")
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(linkParent, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, ".htaccess"), []byte("Options -Indexes\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(linkParent, "storage")); err != nil {
		t.Fatal(err)
	}
	chmodWebTree(home, filepath.Join(linkParent))
	for _, dir := range []string{target, filepath.Join(site, "storage"), filepath.Join(site, "storage", "app")} {
		st, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm()&0111 == 0 {
			t.Fatalf("%s is not executable: %o", dir, st.Mode().Perm())
		}
	}
	ht, err := os.Stat(filepath.Join(target, ".htaccess"))
	if err != nil {
		t.Fatal(err)
	}
	if ht.Mode().Perm()&0444 != 0444 {
		t.Fatalf(".htaccess is not world-readable: %o", ht.Mode().Perm())
	}
	bin := filepath.Join(target, "vp")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	chmodWebTree(home, site)
	st, err := os.Stat(bin)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm()&0111 == 0 || st.Mode().Perm()&0444 != 0444 {
		t.Fatalf("executable file lost its execute bit or is not readable: %o", st.Mode().Perm())
	}
}

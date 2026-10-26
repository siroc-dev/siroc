package api

import (
	"path/filepath"
	"testing"

	"github.com/siroc-dev/siroc/internal/siteopts"
)

func TestReplaceSiteDomainPath(t *testing.T) {
	home := filepath.Join("/home", "alice")
	doc := filepath.Join(home, "domains", "old.test", "public_html")
	got := replaceSiteDomainPath(home, "old.test", "new.test", doc)
	want := filepath.Join(home, "domains", "new.test", "public_html")
	if got != want {
		t.Fatalf("docroot %s", got)
	}
	if replaceSiteDomainPath(home, "old.test", "new.test", filepath.Join(home, "other")) != filepath.Join(home, "other") {
		t.Fatal("left an unrelated path alone")
	}
	if replaceSiteDomainPath(home, "old.test", "new.test", "domains/old.test/public_html") != "domains/new.test/public_html" {
		t.Fatal("relative path")
	}
	opt, changed := replaceSiteOptionPaths(home, "old.test", "new.test", siteopts.Options{
		ComposerRoot: "domains/old.test",
		Cache:        &siteopts.Cache{Path: filepath.Join(home, "domains", "old.test", "cache")},
	})
	if !changed || opt.ComposerRoot != "domains/new.test" {
		t.Fatalf("composer %q", opt.ComposerRoot)
	}
	if opt.Cache == nil || opt.Cache.Path != filepath.Join(home, "domains", "new.test", "cache") {
		t.Fatalf("cache %#v", opt.Cache)
	}
}

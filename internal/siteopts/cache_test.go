package siteopts

import "testing"

func TestSiteCache(t *testing.T) {
	raw, err := Marshal(Options{Cache: &Cache{On: true, Valid: "1h"}})
	if err != nil {
		t.Fatal(err)
	}
	got := Parse(raw)
	if got.Cache == nil || !got.Cache.On || got.Cache.Valid != "" {
		t.Fatalf("default hour stored empty: %#v", got.Cache)
	}
	dir, zone, valid, err := SiteCache(got, "Example.COM")
	if err != nil {
		t.Fatal(err)
	}
	if dir != "/var/cache/nginx/siroc/example.com" || valid != "1h" || zone == "" || zone == DefaultProxyCacheZone {
		t.Fatalf("dir %s zone %s valid %s", dir, zone, valid)
	}
	lines := CacheDirectives(zone, valid)
	if !contains(lines, "proxy_cache "+zone+";") || !contains(lines, "proxy_cache_valid 200 301 302 1h;") {
		t.Fatalf("lines:\n%s", lines)
	}
	loc := InsertCache("    location / {\n        proxy_pass http://127.0.0.1:8080;\n    }\n", lines)
	if !contains(loc, "proxy_cache "+zone+";") || !contains(loc, "proxy_pass http://127.0.0.1:8080;") {
		t.Fatalf("insert:\n%s", loc)
	}
	if InsertCache(loc, lines) != loc {
		t.Fatal("second insert should leave an existing cache")
	}

	kept, err := Normalize(Options{Cache: &Cache{On: false, Valid: "10m", Path: "/mnt/data/nginx-cache"}})
	if err != nil || kept.Cache == nil || kept.Cache.On || kept.Cache.Valid != "10m" || kept.Cache.Path != "/mnt/data/nginx-cache" {
		t.Fatalf("%v %#v", err, kept.Cache)
	}
	if _, _, _, err := SiteCache(kept, "example.com"); err != nil {
		t.Fatal(err)
	}
	off, err := Marshal(Options{Cache: &Cache{On: false}})
	if err != nil {
		t.Fatal(err)
	}
	if !IsZero(Parse(off)) {
		t.Fatal("off cache should be omitted")
	}
	if _, err := Normalize(Options{Cache: &Cache{On: true, Valid: "2h"}}); err == nil {
		t.Fatal("expected invalid cache time")
	}
	if _, err := CachePurgePath("/var/cache/nginx/siroc"); err == nil {
		t.Fatal("shared cache parent should be refused")
	}
	if _, err := CachePurgePath("/etc/nginx"); err == nil {
		t.Fatal("system path should be refused")
	}
}

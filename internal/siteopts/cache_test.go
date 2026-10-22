package siteopts

import "testing"

func TestSiteCache(t *testing.T) {
	raw, err := Marshal(Options{Cache: &Cache{On: true, Valid: "1h", Path: "/var/cache/nginx/siroc/abraventure.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(raw, `"enabled":true`) || contains(raw, `"on":true`) {
		t.Fatalf("stored %s", raw)
	}
	got := Parse(raw)
	if got.Cache == nil || !got.Cache.On || got.Cache.Valid != "" || got.Cache.Path != "/var/cache/nginx/siroc/abraventure.com" {
		t.Fatalf("default hour stored empty: %#v", got.Cache)
	}
	legacy := Parse(`{"cache":{"on":true}}`)
	if legacy.Cache == nil || !legacy.Cache.On {
		t.Fatalf("legacy on flag: %#v", legacy.Cache)
	}
	fromPath := Parse(`{"cache":{"path":"/mnt/data/nginx-cache"}}`)
	if fromPath.Cache == nil || !fromPath.Cache.On || fromPath.Cache.Path != "/mnt/data/nginx-cache" {
		t.Fatalf("path implies on: %#v", fromPath.Cache)
	}
	dir, zone, valid, err := SiteCache(Parse(`{"cache":{"enabled":true}}`), "Example.COM")
	if err != nil {
		t.Fatal(err)
	}
	if dir != "/var/cache/nginx/siroc/example.com" || valid != "1h" || zone == "" || zone == DefaultProxyCacheZone {
		t.Fatalf("dir %s zone %s valid %s", dir, zone, valid)
	}
	lines := CacheDirectives(zone, Cache{})
	if !contains(lines, "proxy_cache "+zone+";") || !contains(lines, "proxy_cache_valid 200 301 302 1h;") || !contains(lines, "proxy_no_cache $http_authorization;") {
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

func TestCacheRecipes(t *testing.T) {
	wp := Cache{
		On: true, Preset: "wordpress", Idle: "7d", Stale: true, Refresh: true,
		Cookies: "wordpress_logged_in, wp-postpass_",
		Skip:    "wp-admin /wp-login.php",
	}
	got, err := Normalize(Options{Cache: &wp})
	if err != nil {
		t.Fatal(err)
	}
	c := *got.Cache
	if c.Cookies != "wordpress_logged_in wp-postpass_" || c.Skip != "/wp-admin /wp-login.php" || c.Idle != "7d" {
		t.Fatalf("%#v", c)
	}
	lines := CacheDirectives("zone", c)
	for _, want := range []string{
		`if ($request_uri ~* "^(/wp-admin(/|$)|/wp-login\.php(/|$))")`,
		`if ($http_cookie ~* "wordpress_logged_in|wp-postpass_")`,
		"proxy_cache_use_stale updating error timeout",
		"proxy_cache_background_update on;",
		"proxy_no_cache $siroc_cache_skip $http_authorization;",
		"proxy_ignore_headers Set-Cookie Cache-Control Expires X-Accel-Expires;",
	} {
		if !contains(lines, want) {
			t.Fatalf("missing %q\n%s", want, lines)
		}
	}
	honor := CacheDirectives("zone", Cache{Honor: true})
	if contains(honor, "Cache-Control") || !contains(honor, "proxy_ignore_headers Set-Cookie;") {
		t.Fatalf("honor:\n%s", honor)
	}
	path := CachePathLine("/var/cache/nginx/siroc/example.com", "zone", Cache{Size: "10g", Idle: "30d"})
	if !contains(path, "max_size=10g") || !contains(path, "inactive=30d") {
		t.Fatalf("path %s", path)
	}
	if _, err := Normalize(Options{Cache: &Cache{On: true, Cookies: "bad cookie!"}}); err == nil {
		t.Fatal("expected a rejected cookie")
	}
	if _, err := Normalize(Options{Cache: &Cache{On: true, Skip: "/../etc"}}); err == nil {
		t.Fatal("expected a rejected skip path")
	}
}

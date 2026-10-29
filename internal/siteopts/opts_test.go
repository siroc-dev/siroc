package siteopts

import "testing"

func TestNormalizeRedirectsAndAccess(t *testing.T) {
	got, err := Normalize(Options{
		Index:  []string{"index.php", "index.php", "index.html"},
		Access: "allow",
		IPs:    []string{"10.0.0.0/8", "203.0.113.5"},
		Redirects: []Redirect{
			{From: "/old", To: "/new", Code: 0},
			{From: "/go", To: "https://example.com/x", Code: 302},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Index) != 2 || got.Index[0] != "index.php" {
		t.Fatalf("index %#v", got.Index)
	}
	if got.Redirects[0].Code != 301 {
		t.Fatalf("code %d", got.Redirects[0].Code)
	}
	extra, err := NginxExtra(got, "")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(extra, "location = /old") || !contains(extra, "return 301 /new;") {
		t.Fatalf("extra:\n%s", extra)
	}
	access, err := NginxAccess(got)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(access, "allow 10.0.0.0/8;") || !contains(access, "deny all;") {
		t.Fatalf("access:\n%s", access)
	}
	idx, err := ApacheIndex(got)
	if err != nil || idx != "index.php index.html" {
		t.Fatalf("index %q %v", idx, err)
	}
}

func TestNormalizePHPAndComposerPath(t *testing.T) {
	on := true
	got, err := Normalize(Options{
		PHP:          &PHP{MemoryLimit: "512M", MaxExecutionTime: 120, DisplayErrors: &on, DisableFunctions: "exec,system"},
		ComposerRoot: "domains/shop",
		LaravelRoot:  "apps/shop",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.PHP == nil || got.PHP.MemoryLimit != "512M" || got.PHP.MaxExecutionTime != 120 || got.PHP.DisplayErrors == nil || !*got.PHP.DisplayErrors {
		t.Fatalf("php %#v", got.PHP)
	}
	if got.ComposerRoot != "domains/shop" {
		t.Fatalf("root %q", got.ComposerRoot)
	}
	if got.LaravelRoot != "apps/shop" {
		t.Fatalf("laravel %q", got.LaravelRoot)
	}
	if _, err := Normalize(Options{PHP: &PHP{MemoryLimit: "lots"}}); err == nil {
		t.Fatal("expected invalid memory_limit")
	}
	if _, err := Normalize(Options{ComposerRoot: "../etc"}); err == nil {
		t.Fatal("expected invalid composer path")
	}
	if _, err := Normalize(Options{LaravelRoot: "../etc"}); err == nil {
		t.Fatal("expected invalid laravel path")
	}
	cleared, err := Normalize(Options{PHP: &PHP{}})
	if err != nil || cleared.PHP != nil {
		t.Fatalf("empty php %#v %v", cleared.PHP, err)
	}
}

func TestRejectUnsafe(t *testing.T) {
	if _, err := Normalize(Options{Redirects: []Redirect{{From: "/a;rm", To: "/b"}}}); err == nil {
		t.Fatal("expected bad path")
	}
	if _, err := Normalize(Options{Redirects: []Redirect{{From: "/.well-known/acme-challenge/x", To: "/b"}}}); err == nil {
		t.Fatal("expected well-known rejection")
	}
	if _, err := Normalize(Options{Access: "allow"}); err == nil {
		t.Fatal("expected empty allow list")
	}
	if _, err := Normalize(Options{IPs: []string{"not-an-ip"}, Access: "deny"}); err == nil {
		t.Fatal("expected bad ip")
	}
	if _, err := NginxExtra(Options{Hotlink: true}, "http://evil; rm"); err == nil {
		t.Fatal("expected bad proxy")
	}
}

func TestMaintenanceAndHotlink(t *testing.T) {
	extra, err := NginxExtra(Options{Maintenance: true, Hotlink: true}, "http://127.0.0.1:3000/")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(extra, "return 503;") || !contains(extra, "acme-challenge") {
		t.Fatalf("maintenance missing:\n%s", extra)
	}
	if !contains(extra, "valid_referers") || !contains(extra, "proxy_pass http://127.0.0.1:3000/;") {
		t.Fatalf("hotlink missing:\n%s", extra)
	}
	withHost, err := NginxExtra(Options{Hotlink: true, HotlinkDomains: []string{"https://CDN.Example.com/img", "*.assets.example.com"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(withHost, "valid_referers none blocked server_names cdn.example.com *.assets.example.com;") {
		t.Fatalf("hotlink domains:\n%s", withHost)
	}
	if _, err := Normalize(Options{HotlinkDomains: []string{"not a domain"}}); err == nil {
		t.Fatal("expected invalid hotlink domain")
	}
}

func TestLogSwitchesStaySeparate(t *testing.T) {
	off := false
	raw, err := Marshal(Options{AccessLog: &off, Hotlink: true})
	if err != nil {
		t.Fatal(err)
	}
	got := Parse(raw)
	if LogOn(got.AccessLog) || !LogOn(got.ErrorLog) || !got.Hotlink {
		t.Fatalf("%#v", got)
	}
	on := true
	raw, err = Marshal(Options{ErrorLog: &on})
	if err != nil {
		t.Fatal(err)
	}
	got = Parse(raw)
	if got.ErrorLog != nil || !IsZero(got) {
		t.Fatalf("on is the default: %#v", got)
	}
}

func TestForceHTTPSKept(t *testing.T) {
	on := true
	raw, err := Marshal(Options{ForceHTTPS: &on, Hotlink: true})
	if err != nil {
		t.Fatal(err)
	}
	got := Parse(raw)
	if got.ForceHTTPS == nil || !*got.ForceHTTPS || !got.Hotlink {
		t.Fatalf("%#v", got)
	}
	off := false
	raw, err = Marshal(Options{ForceHTTPS: &off})
	if err != nil {
		t.Fatal(err)
	}
	got = Parse(raw)
	if got.ForceHTTPS == nil || *got.ForceHTTPS {
		t.Fatalf("%#v", got)
	}
}

func TestStaticDirect(t *testing.T) {
	body, err := NginxStatic(Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"location ~* \\.(ac3|avi|", "|css|", "|js|", "webp|woff|woff2|", "try_files $uri @siroc_apache;", "location @siroc_apache {", "proxy_pass http://127.0.0.1:8080;"} {
		if !contains(body, want) {
			t.Fatalf("missing %q\n%s", want, body)
		}
	}
	off := false
	empty, err := NginxStatic(Options{Static: &off})
	if err != nil || empty != "" {
		t.Fatalf("off: %q %v", empty, err)
	}
	custom, err := NginxStatic(Options{StaticExt: "CSS, js|png"})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(custom, "location ~* \\.(css|js|png)$") || contains(custom, "ac3|") {
		t.Fatalf("custom:\n%s", custom)
	}
	if _, err := NginxStatic(Options{StaticExt: "php"}); err == nil {
		t.Fatal("expected php rejected")
	}
	if _, err := NginxStatic(Options{StaticExt: "css{js"}); err == nil {
		t.Fatal("expected bad extension")
	}
	hot, err := NginxExtra(Options{Hotlink: true}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(hot, "try_files $uri @siroc_apache;") || contains(hot, "proxy_pass") {
		t.Fatalf("hotlink should serve the file:\n%s", hot)
	}
	proxied, err := NginxExtra(Options{Hotlink: true, Static: &off}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(proxied, "proxy_pass http://127.0.0.1:8080;") {
		t.Fatalf("hotlink off static:\n%s", proxied)
	}
}

func contains(s, part string) bool {
	return len(s) >= len(part) && (s == part || len(part) == 0 || (len(s) > 0 && indexOf(s, part) >= 0))
}

func indexOf(s, part string) int {
	for i := 0; i+len(part) <= len(s); i++ {
		if s[i:i+len(part)] == part {
			return i
		}
	}
	return -1
}

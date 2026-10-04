package siteopts

import "testing"

func TestProxyLocation(t *testing.T) {
	got, err := Normalize(Options{Proxy: &Proxy{
		ShowPath:       true,
		Path:           "/",
		Target:         "https://doujinsuki.com",
		Host:           "doujinsuki.com",
		Websocket:      true,
		ConnectTimeout: 60,
		SendTimeout:    600,
		ReadTimeout:    600,
		Rewrites:       []ProxyRewrite{{From: "/aaa", To: "/bbb"}},
		Replacements:   []ProxyReplace{{From: "old", To: "new", Rule: "g"}},
		Cache:          true,
		Gzip:           true,
		Black:          []string{"203.0.113.5"},
		Config:         "proxy_set_header X-Debug 1;\ninclude /tmp/x;",
	}})
	if err == nil {
		t.Fatal("include should be rejected")
	}
	got, err = Normalize(Options{Proxy: &Proxy{
		ShowPath:     true,
		Path:         "/app",
		Target:       "https://doujinsuki.com",
		Host:         "doujinsuki.com",
		Websocket:    true,
		Rewrites:     []ProxyRewrite{{From: "/aaa", To: "/bbb"}},
		Replacements: []ProxyReplace{{From: "old", To: "new", Rule: "g"}},
		Cache:        true,
		Gzip:         true,
		Black:        []string{"203.0.113.5"},
		White:        []string{"192.0.2.10"},
		Config:       "proxy_set_header X-Debug 1;",
	}})
	if err != nil {
		t.Fatal(err)
	}
	loc, err := NginxProxyLocation(got, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"location ^~ /app {",
		"deny 203.0.113.5;",
		"allow 192.0.2.10;",
		"deny all;",
		"rewrite ^/aaa(.*)$ /bbb$1 break;",
		"proxy_pass https://doujinsuki.com;",
		"proxy_set_header Host doujinsuki.com;",
		"proxy_ssl_server_name on;",
		"proxy_connect_timeout 60s;",
		"proxy_cache siroc_cache;",
		"gzip on;",
		"sub_filter 'old' 'new';",
		"sub_filter_once off;",
		"proxy_set_header X-Debug 1;",
		"$connection_upgrade",
	} {
		if !contains(loc, want) {
			t.Fatalf("missing %s\n%s", want, loc)
		}
	}
	legacy, err := NginxProxyLocation(Options{}, "http://127.0.0.1:3000/")
	if err != nil || !contains(legacy, "proxy_pass http://127.0.0.1:3000/;") || !contains(legacy, "proxy_read_timeout 300s;") {
		t.Fatalf("legacy %v\n%s", err, legacy)
	}
}

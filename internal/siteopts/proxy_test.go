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
		"proxy_set_header X-Real-Port $remote_port;",
		"proxy_set_header REMOTE-HOST $remote_addr;",
		"proxy_ssl_server_name on;",
		"proxy_connect_timeout 60s;",
		"proxy_set_header Connection \"upgrade\";",
		"proxy_cache siroc_cache;",
		"gzip on;",
		"sub_filter 'old' 'new';",
		"sub_filter_once off;",
		"proxy_set_header X-Debug 1;",
	} {
		if !contains(loc, want) {
			t.Fatalf("missing %s\n%s", want, loc)
		}
	}
	custom, err := Normalize(Options{Proxy: &Proxy{Target: "http://127.0.0.1:3000/", Cache: true, CachePath: "/mnt/data/nginx-cache"}})
	if err != nil {
		t.Fatal(err)
	}
	if custom.Proxy.CachePath != "/mnt/data/nginx-cache" {
		t.Fatalf("path %s", custom.Proxy.CachePath)
	}
	loc, err = NginxProxyLocation(custom, "")
	if err != nil || !contains(loc, "proxy_cache "+ProxyCacheZone("/mnt/data/nginx-cache")+";") {
		t.Fatalf("custom cache %v\n%s", err, loc)
	}
	if _, err := Normalize(Options{Proxy: &Proxy{Target: "http://127.0.0.1:3000/", Cache: true, CachePath: "/etc/nginx"}}); err == nil {
		t.Fatal("system cache path should be rejected")
	}
	if _, err := Normalize(Options{Proxy: &Proxy{Target: "http://127.0.0.1:3000/", Cache: true, CachePath: "/mnt"}}); err == nil {
		t.Fatal("disk root should be rejected")
	}
	legacy, err := NginxProxyLocation(Options{}, "http://127.0.0.1:3000/")
	if err != nil || !contains(legacy, "proxy_pass http://127.0.0.1:3000;") || !contains(legacy, "proxy_set_header Host $http_host;") {
		t.Fatalf("legacy %v\n%s", err, legacy)
	}
	host, err := NginxProxyLocation(Options{}, "https://doujinfast.com/")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"proxy_pass https://doujinfast.com;",
		"proxy_set_header Host doujinfast.com;",
		"proxy_set_header X-Real-IP $remote_addr;",
		"proxy_set_header X-Real-Port $remote_port;",
		"proxy_set_header X-Forwarded-For $remote_addr;",
		"proxy_set_header CF-Connecting-IP \"\";",
		"proxy_set_header REMOTE-HOST $remote_addr;",
		"proxy_ssl_server_name on;",
		"proxy_connect_timeout 60s;",
		"proxy_send_timeout 600s;",
		"proxy_read_timeout 600s;",
		"proxy_http_version 1.1;",
		"proxy_set_header Upgrade $http_upgrade;",
		"proxy_set_header Connection \"upgrade\";",
	} {
		if !contains(host, want) {
			t.Fatalf("missing %s\n%s", want, host)
		}
	}
	if contains(host, "proxy_pass https://doujinfast.com/") || contains(host, "proxy_ssl_name") {
		t.Fatalf("unexpected proxy line\n%s", host)
	}
}

package software

import "testing"

func TestNginxModuleStale(t *testing.T) {
	msg := `nginx: [emerg] module "/usr/lib/nginx/modules/ndk_http_module.so" version 1031007 instead of 1031008 in /etc/nginx/nginx.conf:1
nginx: configuration file /etc/nginx/nginx.conf test failed`
	if !NginxModuleStale(msg) {
		t.Fatal("expected a stale module")
	}
	if NginxModuleStale("nginx: [emerg] unexpected \"}\" in /etc/nginx/nginx.conf:12") {
		t.Fatal("a syntax error is not a stale module")
	}
}

func TestStripSirocLoadModules(t *testing.T) {
	conf := "user nginx;\nload_module /usr/lib/nginx/modules/ndk_http_module.so;\nload_module /usr/lib/nginx/modules/ngx_http_lua_module.so;\nload_module /usr/lib/nginx/modules/ngx_http_vod_module.so;\nevents {}\n"
	got, changed := stripSirocLoadModules(conf)
	if !changed {
		t.Fatal("expected change")
	}
	if nginxHasDirective(got, sirocLoadModules[1]) {
		t.Fatalf("lua module still active:\n%s", got)
	}
	if !stringsContains(got, "events {}") {
		t.Fatalf("lost events:\n%s", got)
	}
	again, changed := stripSirocLoadModules(got)
	if changed || again != got {
		t.Fatal("second pass")
	}
}

func stringsContains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

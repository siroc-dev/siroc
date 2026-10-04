package software

import "testing"

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

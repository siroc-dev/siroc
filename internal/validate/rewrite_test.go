package validate

import (
	"strings"
	"testing"
)

func TestSplitNginxRewrite(t *testing.T) {
	server, inner, skip, err := SplitNginxRewrite("try_files $uri $uri/ /index.php?$args;")
	if err != nil || skip || server != "" || !strings.Contains(inner, "try_files $uri $uri/ /index.php?$args;") {
		t.Fatalf("legacy inner=%q server=%q skip=%v err=%v", inner, server, skip, err)
	}

	raw := "location /vod/ {\n    vod hls;\n    alias /home/user/videos/;\n    aio threads=default;\n}\n"
	server, inner, skip, err = SplitNginxRewrite(raw)
	if err != nil || skip || inner != "" {
		t.Fatalf("vod inner=%q skip=%v err=%v", inner, skip, err)
	}
	for _, want := range []string{"location /vod/ {", "vod hls;", "alias /home/user/videos/;", "aio threads=default;"} {
		if !strings.Contains(server, want) {
			t.Fatalf("missing %s\n%s", want, server)
		}
	}

	raw = "location / {\n    content_by_lua_block {\n        ngx.say(\"ok\")\n    }\n}\n"
	server, inner, skip, err = SplitNginxRewrite(raw)
	if err != nil || !skip || inner != "" || !strings.Contains(server, "content_by_lua_block {") {
		t.Fatalf("lua server=%q inner=%q skip=%v err=%v", server, inner, skip, err)
	}

	if _, err := NginxSnippet("include /etc/nginx/nginx.conf;"); err == nil {
		t.Fatal("include should be rejected")
	}
	if _, err := NginxSnippet("location / {\n    alias ../etc/;\n}\n"); err == nil {
		t.Fatal("dotdot alias should be rejected")
	}
}

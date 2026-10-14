package hosting

import "testing"

func TestNginxVersionUsesHTTP2Directive(t *testing.T) {
	if nginxVersionUsesHTTP2Directive("nginx version: nginx/1.24.0") {
		t.Fatal("1.24 keeps listen http2")
	}
	if nginxVersionUsesHTTP2Directive("nginx/1.25.0") {
		t.Fatal("1.25.0 keeps listen http2")
	}
	if !nginxVersionUsesHTTP2Directive("nginx version: nginx/1.25.1") {
		t.Fatal("1.25.1 uses http2 on")
	}
	if !nginxVersionUsesHTTP2Directive("nginx/1.27.4") {
		t.Fatal("1.27 uses http2 on")
	}
}

func TestRewriteListenHTTP2(t *testing.T) {
	in := "" +
		"server {\n" +
		"    listen 443 ssl http2;\n" +
		"    listen [::]:443 ssl http2;\n" +
		"    server_name abraventure.com;\n" +
		"}\n"
	got, ok := rewriteListenHTTP2(in)
	if !ok {
		t.Fatal("expected a rewrite")
	}
	want := "" +
		"server {\n" +
		"    listen 443 ssl;\n" +
		"    listen [::]:443 ssl;\n" +
		"    http2 on;\n" +
		"    server_name abraventure.com;\n" +
		"}\n"
	if got != want {
		t.Fatalf("got:\n%s", got)
	}
	again, ok := rewriteListenHTTP2(got)
	if ok || again != got {
		t.Fatalf("second pass changed the file: %v\n%s", ok, again)
	}
}

func TestRewriteListenHTTP2SkipsExistingDirective(t *testing.T) {
	in := "    listen 443 ssl http2;\n    http2 on;\n"
	got, ok := rewriteListenHTTP2(in)
	if !ok {
		t.Fatal("expected listen to be updated")
	}
	if got != "    listen 443 ssl;\n    http2 on;\n" {
		t.Fatalf("got:\n%s", got)
	}
}

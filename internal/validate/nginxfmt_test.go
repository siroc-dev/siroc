package validate

import "testing"

func TestFormatNginx(t *testing.T) {
	in := "server{listen 80;location ~ \\.php$ {proxy_pass http://127.0.0.1:8080;}}"
	got := FormatNginx(in)
	want := "" +
		"server {\n" +
		"    listen 80;\n" +
		"    location ~ \\.php$ {\n" +
		"        proxy_pass http://127.0.0.1:8080;\n" +
		"    }\n" +
		"}\n"
	if got != want {
		t.Fatalf("got:\n%s", got)
	}
	if again := FormatNginx(got); again != got {
		t.Fatalf("second pass changed the file:\n%s", again)
	}
}

func TestFormatNginxKeepsQuotedText(t *testing.T) {
	in := "add_header Content-Security-Policy \"default-src 'self'; script-src 'none'\";\n"
	got := FormatNginx(in)
	want := "add_header Content-Security-Policy \"default-src 'self'; script-src 'none'\";\n"
	if got != want {
		t.Fatalf("got:\n%s", got)
	}
	quotedBrace := "location ~ \"^/foo/{1,2}$\" {\n    return 200;\n}\n"
	if again := FormatNginx(quotedBrace); again != quotedBrace {
		t.Fatalf("brace inside quotes was treated as a block:\n%s", again)
	}
}

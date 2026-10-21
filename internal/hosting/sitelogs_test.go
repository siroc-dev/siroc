package hosting

import (
	"strings"
	"testing"
)

func TestNginxLogLines(t *testing.T) {
	if got := nginxAccessLine("ex.com", true); !strings.Contains(got, "/var/log/nginx/sites/ex.com-access.log combined if=$siroc_asset_log;") {
		t.Fatal(got)
	}
	if got := nginxAccessLine("ex.com", false); got != "    access_log off;" {
		t.Fatal(got)
	}
	if got := nginxErrorLine("ex.com", false); got != "    error_log /dev/null emerg;" {
		t.Fatal(got)
	}
	src := "server {\n    server_name ex.com;\n    access_log /var/log/nginx/sites/ex.com-access.log combined;\n    error_log /var/log/nginx/sites/ex.com-error.log;\n}\n"
	off := applyNginxLogLines(src, nginxAccessLine("ex.com", false), nginxErrorLine("ex.com", true))
	if !strings.Contains(off, "access_log off;") || !strings.Contains(off, "ex.com-error.log") {
		t.Fatalf("off access:\n%s", off)
	}
	again := applyNginxLogLines(off, nginxAccessLine("ex.com", false), nginxErrorLine("ex.com", true))
	if again != off {
		t.Fatalf("second pass changed the file:\n%s", again)
	}
	bare := "server {\n    server_name ex.com;\n}\n"
	added := applyNginxLogLines(bare, nginxAccessLine("ex.com", true), nginxErrorLine("ex.com", false))
	if !strings.Contains(added, "ex.com-access.log combined if=$siroc_asset_log;") || !strings.Contains(added, "error_log /dev/null emerg;") {
		t.Fatalf("insert:\n%s", added)
	}
	kept := quietAssetAccessLogs("    access_log off;\n    access_log /var/log/nginx/sites/ex.com-access.log combined;\n")
	if !strings.Contains(kept, "access_log off;") || !strings.Contains(kept, "combined if=$siroc_asset_log;") {
		t.Fatalf("quiet:\n%s", kept)
	}
	if again := quietAssetAccessLogs(kept); again != kept {
		t.Fatalf("quiet second pass:\n%s", again)
	}
}

func TestApacheLogLines(t *testing.T) {
	src := "<VirtualHost *:80>\n    ServerName ex.com\n    DocumentRoot /var/www\n    ErrorLog ${APACHE_LOG_DIR}/sites/ex.com-error.log\n    CustomLog ${APACHE_LOG_DIR}/sites/ex.com-access.log combined\n</VirtualHost>\n"
	off := applyApacheLogLines(src, apacheErrorLine("ex.com", false), apacheAccessLine("ex.com", true))
	if !strings.Contains(off, "ErrorLog /dev/null") || !strings.Contains(off, "ex.com-access.log combined") {
		t.Fatalf("apache:\n%s", off)
	}
}

func TestWithoutHSTS(t *testing.T) {
	nginx := "server {\n    server_name ex.com;\n    add_header Strict-Transport-Security \"max-age=31536000\" always;\n    location / { proxy_pass http://127.0.0.1:8080; }\n}\n"
	got := withoutHSTS(nginx)
	if strings.Contains(strings.ToLower(got), "add_header strict-transport-security") {
		t.Fatalf("nginx still sets the header:\n%s", got)
	}
	if strings.Count(got, "proxy_hide_header Strict-Transport-Security;") != 1 {
		t.Fatalf("nginx hide:\n%s", got)
	}
	again := withoutHSTS(got)
	if strings.Count(again, "proxy_hide_header Strict-Transport-Security;") != 1 {
		t.Fatalf("nginx hide duplicated:\n%s", again)
	}
	apache := "<VirtualHost 127.0.0.1:8080>\n    DocumentRoot /var/www\n    Header always set Strict-Transport-Security \"max-age=31536000\"\n</VirtualHost>\n"
	got = withoutHSTS(apache)
	if strings.Contains(strings.ToLower(got), "header always set strict-transport-security") {
		t.Fatalf("apache still sets the header:\n%s", got)
	}
	if strings.Count(got, "Header always unset Strict-Transport-Security") != 1 {
		t.Fatalf("apache unset:\n%s", got)
	}
}

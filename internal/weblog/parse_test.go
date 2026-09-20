package weblog

import (
	"strings"
	"testing"
)

func TestParseLaravel(t *testing.T) {
	in := `[2026-09-18 13:46:56] local.ERROR: SQLSTATE[HY000] [1698] Access denied {"exception":"[object] (PDOException(code: 1698): denied)"}
[stacktrace]
#0 /home/a01/app/vendor/laravel/framework/src/Illuminate/Database/Connection.php(797)
#1 /home/a01/app/vendor/laravel/framework/src/Illuminate/Database/Connection.php(412)
[2026-09-18 13:47:01] local.INFO: Job processed
`
	got := ParseLogEntries("laravel", "app", in)
	if len(got) != 2 {
		t.Fatalf("entries %d", len(got))
	}
	if got[0].Level != "info" || got[0].Message != "Job processed" {
		t.Fatalf("newest first: %+v", got[0])
	}
	if got[1].Level != "error" || got[1].Env != "local" || got[1].Time != "2026-09-18 13:46:56" {
		t.Fatalf("error: %+v", got[1])
	}
	if got[1].Message != "SQLSTATE[HY000] [1698] Access denied" || strings.Contains(got[1].Message, "exception") {
		t.Fatalf("message: %q", got[1].Message)
	}
	if got[1].Context == "" || !bytesHasSuffix([]byte(got[1].Context), []byte("Connection.php(412)")) {
		t.Fatalf("stack: %q", got[1].Context)
	}
	c := CountLevels(got)
	if c["error"] != 1 || c["info"] != 1 {
		t.Fatalf("counts %#v", c)
	}
}

func TestParseAccessAndErrors(t *testing.T) {
	acc := ParseLogEntries("nginx", "access", `127.0.0.1 - - [20/Sep/2026:09:46:00 +0000] "GET / HTTP/1.1" 200 123 "-" "siroc"
10.0.0.2 - - [20/Sep/2026:09:46:01 +0000] "GET /missing HTTP/1.1" 404 12
`)
	if len(acc) != 2 || acc[0].Status != 404 || acc[0].Level != "warning" || acc[1].Status != 200 {
		t.Fatalf("%+v", acc)
	}
	ngx := ParseLogEntries("nginx", "error", `2026/09/20 09:46:00 [error] 1#1: *1 open() "/missing" failed (2: No such file)
`)
	if len(ngx) != 1 || ngx[0].Level != "error" || ngx[0].Time != "2026/09/20 09:46:00" {
		t.Fatalf("%+v", ngx)
	}
	ap := ParseLogEntries("apache", "error", `[Sun Sep 20 09:46:00.000000 2026] [core:error] [pid 1] File does not exist: /missing
`)
	if len(ap) != 1 || ap[0].Level != "error" || ap[0].Env != "core" {
		t.Fatalf("%+v", ap)
	}
}

func TestParseWAFAndFilterHost(t *testing.T) {
	in := `--abc123-A--
[20/Sep/2026:09:46:00 +0000] Xid 1.2.3.4 1 127.0.0.1 80
--abc123-B--
GET /?id=1' HTTP/1.1
Host: a01.test
--abc123-H--
Message: Warning. Pattern match at ARGS:id. [id "942100"] [msg "SQL Injection Attack"] [severity "CRITICAL"]
Access denied with code 403
--abc123-Z--
--def456-A--
[20/Sep/2026:09:46:01 +0000] Yid 1.2.3.4 1 127.0.0.1 80
--def456-B--
GET / HTTP/1.1
Host: other.test
--def456-H--
Message: Warning. [id "913100"] [msg "Found User-Agent"] [severity "NOTICE"]
--def456-Z--
`
	got := ParseLogEntries("waf", "audit", in)
	if len(got) != 2 {
		t.Fatalf("entries %d %#v", len(got), got)
	}
	if got[0].Level != "notice" || !strings.Contains(got[0].Message, "Found User-Agent") {
		t.Fatalf("newest %+v", got[0])
	}
	if got[1].Level != "error" || got[1].Env != "id 942100" || !strings.Contains(got[1].Message, "SQL Injection") {
		t.Fatalf("blocked %+v", got[1])
	}
	only := FilterWAFByHost(in, "a01.test")
	if strings.Contains(only, "other.test") || !strings.Contains(only, "a01.test") {
		t.Fatalf("filter %q", only)
	}
	filtered := ParseLogEntries("waf", "audit", only)
	if len(filtered) != 1 || filtered[0].Env != "id 942100" {
		t.Fatalf("filtered %#v", filtered)
	}
}

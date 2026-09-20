package weblog

import "testing"

func TestTailBytes(t *testing.T) {
	got, trunc := TailBytes([]byte("abc"), 100)
	if trunc || string(got) != "abc" {
		t.Fatalf("%q %v", got, trunc)
	}
	in := []byte("keep\n" + stringsRepeat("x", 200) + "\nend\n")
	got, trunc = TailBytes(in, 20)
	if !trunc || !bytesHasSuffix(got, []byte("end\n")) {
		t.Fatalf("%q trunc=%v", got, trunc)
	}
}

func TestInHome(t *testing.T) {
	if !InHome("/home/a01", "/home/a01/domains/a01.test") {
		t.Fatal("child")
	}
	if InHome("/home/a01", "/home/a01/../a02") {
		t.Fatal("escape")
	}
	if InHome("/home/a01", "/var/log/nginx") {
		t.Fatal("outside")
	}
}

func TestParseSiteLogID(t *testing.T) {
	g, n, ok := ParseSiteLogID("waf-audit")
	if !ok || g != "waf" || n != "audit" {
		t.Fatalf("waf %s %s %v", g, n, ok)
	}
	g, n, ok = ParseSiteLogID("nginx-error")
	if !ok || g != "nginx" || n != "error" {
		t.Fatalf("%s %s %v", g, n, ok)
	}
	g, n, ok = ParseSiteLogID("laravel:laravel-2026-09-20.log")
	if !ok || g != "laravel" || n != "laravel-2026-09-20.log" {
		t.Fatalf("%s %s %v", g, n, ok)
	}
	if _, _, ok = ParseSiteLogID("laravel:../etc/passwd"); ok {
		t.Fatal("escape")
	}
	if ValidLaravelLogName("bad name.log") || ValidLaravelLogName("../x.log") {
		t.Fatal("invalid name accepted")
	}
}

func stringsRepeat(s string, n int) string {
	b := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		b = append(b, s...)
	}
	return string(b)
}

func bytesHasSuffix(b, suf []byte) bool {
	if len(b) < len(suf) {
		return false
	}
	return string(b[len(b)-len(suf):]) == string(suf)
}

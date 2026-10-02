package weblog

import "testing"

func TestLookupSystemLog(t *testing.T) {
	s, ok := lookupSystemLog("panel-auth")
	if !ok || s.Path != "/var/log/siroc/auth.log" || s.Group != "panel" {
		t.Fatalf("%#v %v", s, ok)
	}
	if _, ok := lookupSystemLog("../passwd"); ok {
		t.Fatal("reject unknown id")
	}
}

func TestPHPFpmLogID(t *testing.T) {
	id, ok := phpFpmLogID("php8.3-fpm.log")
	if !ok || id != "php-fpm-8.3" {
		t.Fatalf("%q %v", id, ok)
	}
	if _, ok := phpFpmLogID("../x-fpm.log"); ok {
		t.Fatal("reject")
	}
}

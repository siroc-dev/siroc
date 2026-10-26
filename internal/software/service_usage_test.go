package software

import "testing"

func TestParseServiceUsage(t *testing.T) {
	text := "Id=nginx.service\nMemoryCurrent=1048576\nTasksCurrent=3\nControlGroup=/system.slice/nginx.service\n\nId=php8.4-fpm.service\nMemoryCurrent=[not set]\nTasksCurrent=8\nControlGroup=/system.slice/php8.4-fpm.service\n"
	got := ParseServiceUsage(text)
	if got["nginx"].Memory != 1048576 || got["nginx"].Processes != 3 {
		t.Fatalf("nginx %#v", got["nginx"])
	}
	if got["php8.4-fpm"].Memory != 0 || got["php8.4-fpm"].Processes != 8 {
		t.Fatalf("php %#v", got["php8.4-fpm"])
	}
	cg := ParseControlGroups(text)
	if cg["nginx"] != "/system.slice/nginx.service" {
		t.Fatal(cg)
	}
	if ValidServiceUnit("nginx") && ValidServiceUnit("php8.4-fpm") && !ValidServiceUnit("nginx;rm") {
	} else {
		t.Fatal("unit")
	}
}

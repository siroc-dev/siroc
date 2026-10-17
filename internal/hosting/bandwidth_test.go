package hosting

import (
	"strings"
	"testing"
)

func TestAddBandwidthLog(t *testing.T) {
	src := "server {\n    listen 80;\n    server_name ex.com;\n}\nserver {\n    listen 443 ssl;\n    server_name ex.com;\n}\n"
	got := AddBandwidthLog(src, "ex.com")
	want := "    access_log /var/log/nginx/sites/ex.com-bw.log siroc_bw;"
	if strings.Count(got, want) != 2 {
		t.Fatalf("both server blocks should count bytes:\n%s", got)
	}
	if strings.Contains(got, "server_name /var/log") {
		t.Fatalf("server_name was treated as a server block:\n%s", got)
	}
	if AddBandwidthLog(got, "ex.com") != got {
		t.Fatal("adding the bandwidth log twice changed the file")
	}
	if AddBandwidthLog(src, "../etc/passwd") != src {
		t.Fatal("a path domain should be ignored")
	}
}

func TestSumLines(t *testing.T) {
	sum, consumed, err := SumLines(strings.NewReader("10\n20\n"))
	if err != nil || sum != 30 || consumed != 6 {
		t.Fatalf("sum %d consumed %d err %v", sum, consumed, err)
	}
	sum, consumed, err = SumLines(strings.NewReader("5\nbad\n7\n12"))
	if err != nil || sum != 12 || consumed != 8 {
		t.Fatalf("partial sum %d consumed %d err %v", sum, consumed, err)
	}
}

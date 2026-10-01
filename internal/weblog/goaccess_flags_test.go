package weblog

import (
	"strings"
	"testing"
)

func TestGoaccessReportArgs(t *testing.T) {
	help := "keep-last geoip-database enable-geoip enable-panel"
	args := goaccessReportArgs(help, []string{"/var/log/a.log"}, "/tmp/a.html", "90", "site.test stats", []string{"/geo/city.mmdb", "/geo/asn.mmdb"}, nil)
	joined := strings.Join(args, " ")
	for _, need := range []string{
		"--keep-last",
		"--enable-geoip=mmdb",
		"--geoip-database=/geo/city.mmdb",
		"--geoip-database=/geo/asn.mmdb",
		"--enable-panel=VISIT_TIMES",
		"--enable-panel=GEO_LOCATION",
		"--enable-panel=ASN",
		"--enable-panel=AI_CRAWLERS",
	} {
		if !strings.Contains(joined, need) {
			t.Fatalf("missing %s in %s", need, joined)
		}
	}
	args = goaccessReportArgs(help, []string{"/var/log/a.log"}, "/tmp/a.html", "90", "site.test stats", []string{"/geo/city.mmdb"}, map[string]bool{"AI_CRAWLERS": true})
	joined = strings.Join(args, " ")
	if strings.Contains(joined, "AI_CRAWLERS") {
		t.Fatal(joined)
	}
}

func TestParseGoaccessVersion(t *testing.T) {
	if v := parseGoaccessVersion("GoAccess - 1.9.4.\nFor more details"); v != "1.9.4" {
		t.Fatal(v)
	}
	if !goaccessAtLeast("GoAccess - 1.9.4.", "1.9.4") || goaccessAtLeast("GoAccess - 1.8.1.", "1.9.4") {
		t.Fatal("compare")
	}
}

func TestGoaccessSkipFromError(t *testing.T) {
	skip := map[string]bool{}
	if !goaccessSkipFromError("Unknown module AI_CRAWLERS", skip) || !skip["AI_CRAWLERS"] {
		t.Fatalf("%v", skip)
	}
}

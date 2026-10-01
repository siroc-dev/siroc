package weblog

import (
	"strings"
	"testing"
	"time"
)

func TestDBIPLiteURL(t *testing.T) {
	u := dbipLiteURL("city", "2026-10")
	if !strings.Contains(u, "dbip-city-lite-2026-10.mmdb.gz") {
		t.Fatal(u)
	}
	u = dbipLiteURL("asn", "2026-09")
	if !strings.Contains(u, "dbip-asn-lite-2026-09.mmdb.gz") {
		t.Fatal(u)
	}
}

func TestGeoipMonths(t *testing.T) {
	got := geoipMonths(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if len(got) != 2 || got[0] != "2026-10" || got[1] != "2026-09" {
		t.Fatalf("%v", got)
	}
}

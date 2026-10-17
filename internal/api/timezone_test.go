package api

import "testing"

func TestCleanTimezone(t *testing.T) {
	if got := cleanTimezone("Asia/Bangkok\n"); got != "Asia/Bangkok" {
		t.Fatalf("bangkok: %q", got)
	}
	if got := cleanTimezone("UTC"); got != "UTC" {
		t.Fatalf("utc: %q", got)
	}
	if got := cleanTimezone("Not/AZone"); got != "" {
		t.Fatalf("invalid: %q", got)
	}
	if got := cleanTimezone("Asia/Bangkok\nUTC"); got != "" {
		t.Fatalf("multiline: %q", got)
	}
}

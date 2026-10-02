//go:build linux

package update

import "testing"

func TestCheckTimerIs0400(t *testing.T) {
	if checkCal != "*-*-* 04:00:00" {
		t.Fatalf("daily check must be 04:00, got %q", checkCal)
	}
	if checkUnit != "siroc-update-check" {
		t.Fatalf("unit=%q", checkUnit)
	}
}

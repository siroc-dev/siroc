//go:build linux

package update

import "testing"

func TestCheckTimerIs0400(t *testing.T) {
	if checkCal != "hourly" {
		t.Fatalf("update check must be hourly, got %q", checkCal)
	}
	if checkUnit != "siroc-update-check" {
		t.Fatalf("unit=%q", checkUnit)
	}
}

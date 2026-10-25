package dbmgmt

import "testing"

func TestDBShownValueUsesLiveUntilRAMChanges(t *testing.T) {
	if dbShownValue("512M", "256M", "1G", false) != "512M" {
		t.Fatal("value should be the live setting")
	}
	if dbShownValue("512M", "256M", "1G", true) != "1G" {
		t.Fatal("changing RAM should fill the preset")
	}
	if dbShownValue("", "256M", "1G", false) != "256M" {
		t.Fatal("file value is the current setting when the server is stopped")
	}
}

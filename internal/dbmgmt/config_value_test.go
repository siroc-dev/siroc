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

func TestNormalizeDBTimezone(t *testing.T) {
	got, ok := normalizeDBTimezone(" system ")
	if !ok || got != "SYSTEM" || dbTimezoneNeedsTables(got) {
		t.Fatalf("SYSTEM: %q %v", got, ok)
	}
	got, ok = normalizeDBTimezone("'+07:00'")
	if !ok || got != "+07:00" || mysqlTimezoneConf(got) != "'+07:00'" || dbTimezoneNeedsTables(got) {
		t.Fatalf("offset: %q %v %s", got, ok, mysqlTimezoneConf(got))
	}
	got, ok = normalizeDBTimezone("Asia/Bangkok")
	if !ok || mysqlTimezoneConf(got) != "'Asia/Bangkok'" || !dbTimezoneNeedsTables(got) {
		t.Fatalf("name: %q", got)
	}
	if _, ok := normalizeDBTimezone("Asia/Bangkok;DROP"); ok {
		t.Fatal("rejected a timezone with a separator")
	}
	if mysqlTimezoneConf("UTC") != "'UTC'" {
		t.Fatal("UTC is a named zone")
	}
}

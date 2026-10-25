package software

import "testing"

func TestWebShownValueUsesCurrentUntilRAMChanges(t *testing.T) {
	if webShownValue("on", "off", false) != "on" {
		t.Fatal("open the page with the current value")
	}
	if webShownValue("on", "off", true) != "off" {
		t.Fatal("changing RAM fills the preset")
	}
	if webShownValue("", "1024", false) != "1024" {
		t.Fatal("missing current falls back to the preset")
	}
}

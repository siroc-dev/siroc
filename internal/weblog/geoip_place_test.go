package weblog

import "testing"

func TestFormatPlace(t *testing.T) {
	p := formatPlace("Bangkok", "Thailand", 7470, "TRUE INTERNET")
	if p.Location != "Bangkok, Thailand" || p.ASN != "AS7470" || p.ASOrg != "TRUE INTERNET" {
		t.Fatalf("%+v", p)
	}
	if got := formatPlace("", "Thailand", 0, ""); got.Location != "Thailand" || got.ASN != "" {
		t.Fatalf("%+v", got)
	}
}

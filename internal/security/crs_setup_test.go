package security

import "testing"

func TestParseCRSSetupVersion(t *testing.T) {
	text := `SecAction \
  "id:900990,\
   phase:1,\
   nolog,\
   pass,\
   t:none,\
   setvar:tx.crs_setup_version=335"`
	if parseCRSSetupVersion(text) != 335 {
		t.Fatal(parseCRSSetupVersion(text))
	}
	if parseCRSSetupVersion("setvar:tx.crs_setup_version=400") != 400 {
		t.Fatal("crs 4")
	}
	if parseCRSSetupVersion("no version here") != 0 {
		t.Fatal("empty")
	}
}

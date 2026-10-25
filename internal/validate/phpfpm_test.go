package validate

import "testing"

func TestPHPFPMDynamicSpareOrder(t *testing.T) {
	err := PHPFPM("dynamic", 20, 5, 1, 4, 500, 60, 60)
	if err == nil {
		t.Fatal("start servers above max spare should fail")
	}
	if err := PHPFPM("dynamic", 20, 1, 2, 4, 500, 60, 60); err == nil {
		t.Fatal("start servers below min spare should fail")
	}
	if err := PHPFPM("dynamic", 20, 2, 1, 4, 500, 60, 60); err != nil {
		t.Fatal(err)
	}
	if err := PHPFPM("ondemand", 20, 5, 1, 4, 500, 60, 60); err != nil {
		t.Fatal(err)
	}
}

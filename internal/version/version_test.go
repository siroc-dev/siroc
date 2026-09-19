package version

import "testing"

func TestCompare(t *testing.T) {
	if Compare("0.2.0", "0.2.1") >= 0 {
		t.Fatal("0.2.0 should be older than 0.2.1")
	}
	if Compare("1.0.0", "0.9.9") <= 0 {
		t.Fatal("1.0.0 should be newer")
	}
	if Compare("v0.2.0", "0.2.0") != 0 {
		t.Fatal("v prefix should be ignored")
	}
}

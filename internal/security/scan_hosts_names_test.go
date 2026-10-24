package security

import "testing"

func TestKeepScanHost(t *testing.T) {
	for _, n := range []string{"pma.cp.local", "app.test", "db.local", "shop.localhost"} {
		if !keepScanHost(n) {
			t.Fatalf("kept %s", n)
		}
	}
	for _, n := range []string{"doujinfast.com", "doujinfast.net", "ecchi-doujin.com", "xn--82c5cpkh7basd8hwe.com"} {
		if keepScanHost(n) {
			t.Fatalf("public name was pinned: %s", n)
		}
	}
}

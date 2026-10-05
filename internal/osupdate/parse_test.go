package osupdate

import "testing"

func TestParseUpgradable(t *testing.T) {
	out := `
WARNING: apt does not have a stable CLI interface. Use with caution in scripts.

Listing...
nginx/noble-updates 1.24.0-2ubuntu7.5 amd64 [upgradable from: 1.24.0-2ubuntu7.1]
openssl/noble-updates,noble-security 3.0.13-0ubuntu3.5 amd64 [upgradable from: 3.0.13-0ubuntu3.2]
not a package line
`
	pkgs := ParseUpgradable(out)
	if len(pkgs) != 2 {
		t.Fatalf("got %d packages", len(pkgs))
	}
	if pkgs[0].Name != "nginx" || pkgs[0].Current != "1.24.0-2ubuntu7.1" || pkgs[0].Available != "1.24.0-2ubuntu7.5" || pkgs[0].Source != "noble-updates" {
		t.Fatalf("nginx: %+v", pkgs[0])
	}
	if pkgs[1].Name != "openssl" || pkgs[1].Source != "noble-updates,noble-security" {
		t.Fatalf("openssl: %+v", pkgs[1])
	}
	if len(ParseUpgradable("Listing...\n")) != 0 {
		t.Fatal("empty list")
	}
	if Summarize(0) != "No packages to upgrade." || Summarize(1) != "1 package can be upgraded." || Summarize(3) != "3 packages can be upgraded." {
		t.Fatal(Summarize(3))
	}
}

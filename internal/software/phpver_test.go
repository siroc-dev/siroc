package software

import "testing"

func TestPHPVersionsInclude74(t *testing.T) {
	if !SupportedPHP("7.4") || !SupportedPHP("8.4") {
		t.Fatal(PHPVersions())
	}
	if SupportedPHP("5.6") || SupportedPHP("8.0") {
		t.Fatal("unexpected version accepted")
	}
	if PHPVersions()[len(PHPVersions())-1] != BasePHPVersion {
		t.Fatal("newest listed version should stay the base install")
	}
}

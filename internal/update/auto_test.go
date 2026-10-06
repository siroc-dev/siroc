package update

import (
	"path/filepath"
	"testing"
)

func TestAutoEnabled(t *testing.T) {
	dir := t.TempDir()
	autoUpdateFile = filepath.Join(dir, "auto-update")
	if !AutoEnabled() {
		t.Fatal("missing file should leave automatic updates on")
	}
	if err := writeAuto(false); err != nil {
		t.Fatal(err)
	}
	if AutoEnabled() {
		t.Fatal("off file should disable automatic updates")
	}
	if err := writeAuto(true); err != nil {
		t.Fatal(err)
	}
	if !AutoEnabled() {
		t.Fatal("on file should enable automatic updates")
	}
}

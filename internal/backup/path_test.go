package backup

import "testing"

func TestRestorePathOK(t *testing.T) {
	if !RestorePathOK("/var/backups/siroc/alice/alice-1.tar.gz", "alice") {
		t.Fatal("siroc path")
	}
	if !RestorePathOK("/var/backups/offsite/alice.tar.gz", "") {
		t.Fatal("local dest path")
	}
	if !RestorePathOK("/home/alice/alice.tar.gz", "alice") {
		t.Fatal("home path")
	}
	if RestorePathOK("/etc/passwd", "alice") {
		t.Fatal("system path")
	}
	if RestorePathOK("/var/backups/../etc/passwd", "alice") {
		t.Fatal("dotdot")
	}
}

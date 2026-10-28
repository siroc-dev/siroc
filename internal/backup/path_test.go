package backup

import "testing"

func TestRestorePathOK(t *testing.T) {
	if !RestorePathOK("/var/backups/siroc/alice/alice-1.tar.gz", "alice") {
		t.Fatal("siroc path")
	}
	if !RestorePathOK("/var/backups/offsite/alice.tar.gz", "") {
		t.Fatal("local dest path")
	}
	got, err := UserLocalDir("alice", "/home/alice/backups")
	if err != nil || got != "/home/alice/backups" {
		t.Fatalf("%s %v", got, err)
	}
	if got, err = UserLocalDir("alice", "backups"); err != nil || got != "/home/alice/backups" {
		t.Fatalf("relative %s %v", got, err)
	}
	if _, err = UserLocalDir("alice", "/var/backups/offsite"); err == nil {
		t.Fatal("outside home")
	}
	if _, err = UserLocalDir("alice", "/home/alice/../bob"); err == nil {
		t.Fatal("escaped home")
	}
	if !RestorePathOK("/home/alice/alice.tar.gz", "alice") {
		t.Fatal("home path")
	}
	if !RestorePathOK("/mnt/data/siroc-tmp/da-site.tar.gz", "") {
		t.Fatal("spacious tmp")
	}
	if !RestorePathOK("/siroc-tmp/da-site.tar.gz", "") {
		t.Fatal("root tmp")
	}
	if RestorePathOK("/siroc-tmp", "") {
		t.Fatal("tmp directory itself")
	}
	if !RestorePathOK("/home/tmp/da-site.tar.gz", "") {
		t.Fatal("home tmp")
	}
	if RestorePathOK("/home/tmp2/da-site.tar.gz", "") {
		t.Fatal("similar directory")
	}
	if RestorePathOK("/etc/passwd", "alice") {
		t.Fatal("system path")
	}
	if RestorePathOK("/var/backups/../etc/passwd", "alice") {
		t.Fatal("dotdot")
	}
}

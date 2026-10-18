package backup

import "testing"

func TestRestoreArchiveName(t *testing.T) {
	got, err := RestoreArchiveName(`C:\backups\user.alice.tar.zst`)
	if err != nil || got != "user.alice.tar.zst" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := RestoreArchiveName("../etc/passwd"); err == nil {
		t.Fatal("expected reject")
	}
	if _, err := RestoreArchiveName("notes.txt"); err == nil {
		t.Fatal("expected reject")
	}
	got, err = RestoreArchiveName("my backup (1).tar.gz")
	if err != nil || got != "my_backup__1_.tar.gz" {
		t.Fatalf("%q %v", got, err)
	}
}

package backup

import (
	"strings"
	"testing"
)

func TestFTPMkdirQuotes(t *testing.T) {
	got := ftpMkdirQuotes("/backups/monday/blog.tar.gz")
	want := []string{"--quote", "*MKD /backups", "--quote", "*MKD /backups/monday"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("quotes: %#v", got)
	}
	if ftpMkdirQuotes("/blog.tar.gz") != nil {
		t.Fatal("file in the login directory needs no folder")
	}
	if ftpMkdirQuotes("/backups/../monday/blog.tar.gz") != nil {
		t.Fatal("parent segments are not created")
	}
}

func TestRestorePathOKRejectsHomeOtherUser(t *testing.T) {
	if RestorePathOK("/home/bob/alice.tar.gz", "alice") {
		t.Fatal("other user home")
	}
}

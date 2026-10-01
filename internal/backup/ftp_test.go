package backup

import "testing"

func TestRestorePathOKRejectsHomeOtherUser(t *testing.T) {
	if RestorePathOK("/home/bob/alice.tar.gz", "alice") {
		t.Fatal("other user home")
	}
}

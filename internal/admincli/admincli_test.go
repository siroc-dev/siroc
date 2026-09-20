package admincli

import (
	"path/filepath"
	"testing"

	"github.com/siroc-dev/siroc/internal/auth"
	"github.com/siroc-dev/siroc/internal/store"
)

func TestResetAdminGeneratesPassword(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	hash, err := auth.Hash("old-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreatePanelUser("rootadmin", hash, "admin"); err != nil {
		t.Fatal(err)
	}
	user, pass, err := ResetAdmin(st, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if user != "rootadmin" {
		t.Fatalf("user=%q", user)
	}
	if len(pass) < 8 {
		t.Fatalf("generated password too short: %q", pass)
	}
	got, err := st.GetPanelUserByName("rootadmin")
	if err != nil {
		t.Fatal(err)
	}
	if !auth.Check(got.PasswordHash, pass) {
		t.Fatal("new password was not stored")
	}
	if auth.Check(got.PasswordHash, "old-password") {
		t.Fatal("old password still works")
	}
}

func TestResetAdminRejectsHostUser(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	hash, _ := auth.Hash("host-pass-1")
	if err := st.CreatePanelUser("webuser", hash, "user"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ResetAdmin(st, "webuser", "new-pass-99"); err == nil {
		t.Fatal("expected error for hosting user")
	}
}

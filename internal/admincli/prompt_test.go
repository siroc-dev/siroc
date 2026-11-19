package admincli

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/siroc-dev/siroc/internal/auth"
	"github.com/siroc-dev/siroc/internal/store"
)

func openAdminDB(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func addUser(t *testing.T, st *store.Store, name, password, role string) {
	t.Helper()
	hash, err := auth.Hash(password)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreatePanelUser(name, hash, role); err != nil {
		t.Fatal(err)
	}
}

func stillHas(t *testing.T, st *store.Store, name, password string) {
	t.Helper()
	got, err := st.GetPanelUserByName(name)
	if err != nil {
		t.Fatal(err)
	}
	if !auth.Check(got.PasswordHash, password) {
		t.Fatalf("password for %s changed", name)
	}
}

func TestResetInteractiveGeneratesPassword(t *testing.T) {
	st := openAdminDB(t)
	addUser(t, st, "rootadmin", "old-password", "admin")
	var out bytes.Buffer
	user, pass, err := ResetInteractive(st, "", strings.NewReader("y\n"), &out, func() (string, error) {
		return "", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if user != "rootadmin" || len(pass) < 8 {
		t.Fatalf("user=%q pass=%q", user, pass)
	}
	if !strings.Contains(out.String(), "Reset password for rootadmin?") {
		t.Fatalf("prompt missing name: %s", out.String())
	}
	got, err := st.GetPanelUserByName("rootadmin")
	if err != nil {
		t.Fatal(err)
	}
	if !auth.Check(got.PasswordHash, pass) || auth.Check(got.PasswordHash, "old-password") {
		t.Fatal("stored password was not replaced")
	}
}

func TestResetInteractiveCancelLeavesPassword(t *testing.T) {
	st := openAdminDB(t)
	addUser(t, st, "rootadmin", "old-password", "admin")
	_, _, err := ResetInteractive(st, "", strings.NewReader("n\n"), &bytes.Buffer{}, func() (string, error) {
		t.Fatal("password prompt should not run after cancel")
		return "", nil
	})
	if !errors.Is(err, ErrAborted) {
		t.Fatalf("err=%v", err)
	}
	stillHas(t, st, "rootadmin", "old-password")
}

func TestResetInteractiveConfirmTypedPassword(t *testing.T) {
	st := openAdminDB(t)
	addUser(t, st, "rootadmin", "old-password", "admin")
	answers := []string{"new-pass-99", "new-pass-99"}
	user, pass, err := ResetInteractive(st, "rootadmin", strings.NewReader("yes\n"), &bytes.Buffer{}, func() (string, error) {
		next := answers[0]
		answers = answers[1:]
		return next, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if user != "rootadmin" || pass != "new-pass-99" {
		t.Fatalf("user=%q pass=%q", user, pass)
	}
	got, err := st.GetPanelUserByName("rootadmin")
	if err != nil {
		t.Fatal(err)
	}
	if !auth.Check(got.PasswordHash, "new-pass-99") {
		t.Fatal("typed password was not stored")
	}
}

func TestResetInteractiveRejectsMismatchAndShort(t *testing.T) {
	st := openAdminDB(t)
	addUser(t, st, "rootadmin", "old-password", "admin")
	_, _, err := ResetInteractive(st, "", strings.NewReader("y\n"), &bytes.Buffer{}, func() (string, error) {
		return "short", nil
	})
	if err == nil || !strings.Contains(err.Error(), "at least 8") {
		t.Fatalf("err=%v", err)
	}
	stillHas(t, st, "rootadmin", "old-password")

	answers := []string{"new-pass-99", "other-pass"}
	_, _, err = ResetInteractive(st, "", strings.NewReader("y\n"), &bytes.Buffer{}, func() (string, error) {
		next := answers[0]
		answers = answers[1:]
		return next, nil
	})
	if err == nil || !strings.Contains(err.Error(), "do not match") {
		t.Fatalf("err=%v", err)
	}
	stillHas(t, st, "rootadmin", "old-password")
}

func TestResetInteractiveMultipleAdmins(t *testing.T) {
	st := openAdminDB(t)
	addUser(t, st, "ann", "old-password", "admin")
	addUser(t, st, "ben", "old-password", "admin")

	_, _, err := ResetInteractive(st, "", strings.NewReader("\n"), &bytes.Buffer{}, func() (string, error) {
		t.Fatal("password prompt should not run")
		return "", nil
	})
	if !errors.Is(err, ErrAborted) {
		t.Fatalf("err=%v", err)
	}
	stillHas(t, st, "ann", "old-password")
	stillHas(t, st, "ben", "old-password")

	_, _, err = ResetInteractive(st, "", strings.NewReader("nope\n"), &bytes.Buffer{}, func() (string, error) {
		t.Fatal("password prompt should not run")
		return "", nil
	})
	if err == nil || !strings.Contains(err.Error(), "not a panel administrator") {
		t.Fatalf("err=%v", err)
	}

	var listed bytes.Buffer
	user, _, err := ResetInteractive(st, "", strings.NewReader("ben\n"), &listed, func() (string, error) {
		return "ben-pass-99", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if user != "ben" {
		t.Fatalf("user=%q", user)
	}
	if !strings.Contains(listed.String(), "ann") || !strings.Contains(listed.String(), "ben") {
		t.Fatalf("list missing admins: %s", listed.String())
	}
	stillHas(t, st, "ann", "old-password")
	got, err := st.GetPanelUserByName("ben")
	if err != nil {
		t.Fatal(err)
	}
	if !auth.Check(got.PasswordHash, "ben-pass-99") {
		t.Fatal("ben password was not stored")
	}
}

func TestResetInteractiveRejectsHostUser(t *testing.T) {
	st := openAdminDB(t)
	addUser(t, st, "webuser", "host-pass-1", "user")
	_, _, err := ResetInteractive(st, "webuser", strings.NewReader("y\n"), &bytes.Buffer{}, func() (string, error) {
		t.Fatal("password prompt should not run")
		return "", nil
	})
	if err == nil || !strings.Contains(err.Error(), "not a panel administrator") {
		t.Fatalf("err=%v", err)
	}
}

package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/siroc-dev/siroc/internal/auth"
	"github.com/siroc-dev/siroc/internal/config"
	"github.com/siroc-dev/siroc/internal/store"
)

func TestFormLoginRedirectsAfterSuccess(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	hash, err := auth.Hash("secretpass")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreatePanelUser("admin", hash, "admin"); err != nil {
		t.Fatal(err)
	}
	srv := &Server{Cfg: config.Config{DisableTLS: true}, Store: st, Auth: auth.New(st)}
	form := url.Values{"username": {"admin"}, "password": {"secretpass"}}
	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.login(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Location") != "/" {
		t.Fatalf("location %q", rec.Header().Get("Location"))
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("session cookie was not set")
	}
}

func TestFormLoginRedirectsAfterFailure(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	srv := &Server{Cfg: config.Config{DisableTLS: true}, Store: st, Auth: auth.New(st)}
	form := url.Values{"username": {"admin"}, "password": {"secretpass"}}
	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.login(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status %d", rec.Code)
	}
	if rec.Header().Get("Location") != "/login?error=invalid+credentials" {
		t.Fatalf("location %q", rec.Header().Get("Location"))
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("failed login set a cookie")
	}
}

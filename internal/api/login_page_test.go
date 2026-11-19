package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginPageIsARealPasswordForm(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/login?error=invalid+credentials", nil)
	rec := httptest.NewRecorder()
	loginPage(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`method="post"`,
		`action="/api/login"`,
		`name="username"`,
		`autocomplete="username"`,
		`name="password"`,
		`type="password"`,
		`autocomplete="current-password"`,
		`type="submit"`,
		"invalid credentials",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("page missing %s", want)
		}
	}
	if strings.Contains(body, "<script") && strings.Index(body, `name="password"`) > strings.Index(body, "<script") {
		t.Fatal("password field is created by script instead of the document")
	}
}

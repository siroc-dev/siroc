package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientIPStripsPort(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	r.RemoteAddr = "203.0.113.9:54321"
	if got := ClientIP(r); got != "203.0.113.9" {
		t.Fatalf("got %q", got)
	}
}

func TestAuthFailLineFormat(t *testing.T) {
	line := "2026-10-02 00:04:00 siroc login failed ip=203.0.113.9 user=admin"
	if !strings.Contains(line, "siroc login failed ip=203.0.113.9") {
		t.Fatalf("line=%q", line)
	}
}

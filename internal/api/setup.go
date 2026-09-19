package api

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/siroc-dev/siroc/internal/auth"
	"github.com/siroc-dev/siroc/internal/setup"
)

func (s *Server) prepareFirstRun() {
	n, err := s.Store.PanelUserCount()
	if err != nil {
		log.Printf("setup: cannot count panel users: %v", err)
		return
	}
	if n > 0 {
		setup.Clear(s.Cfg.DataDir)
		return
	}
	tok, err := setup.Ensure(s.Cfg.DataDir)
	if err != nil {
		log.Printf("setup: cannot create first-run token: %v", err)
		return
	}
	url := setup.PublicURL(s.Cfg.ListenAddr, !s.Cfg.DisableTLS, tok)
	if err := setup.WriteURL(s.Cfg.DataDir, url); err != nil {
		log.Printf("setup: cannot write setup url: %v", err)
	}
	log.Printf("First-run admin setup (token-protected): %s", url)
}

func setupTokenFrom(r *http.Request, bodyToken string) string {
	if t := strings.TrimSpace(bodyToken); t != "" {
		return t
	}
	if t := strings.TrimSpace(r.URL.Query().Get("token")); t != "" {
		return t
	}
	if t := strings.TrimSpace(r.Header.Get("X-Setup-Token")); t != "" {
		return t
	}
	return ""
}

func (s *Server) setupStatus(w http.ResponseWriter, r *http.Request) {
	n, err := s.Store.PanelUserCount()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	needed := n == 0
	authorized := false
	if needed {
		authorized = setup.Valid(s.Cfg.DataDir, setupTokenFrom(r, ""))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"needed":     needed,
		"authorized": authorized,
	})
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	n, err := s.Store.PanelUserCount()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if n > 0 {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("already set up"))
		return
	}
	ip := r.RemoteAddr
	if !s.Auth.AllowLogin(ip) {
		writeErr(w, http.StatusTooManyRequests, fmt.Errorf("too many setup attempts"))
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Token    string `json:"token"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !setup.Valid(s.Cfg.DataDir, setupTokenFrom(r, body.Token)) {
		s.Auth.Fail(ip)
		writeErr(w, http.StatusForbidden, fmt.Errorf("invalid or missing setup token"))
		return
	}
	if !auth.ValidUsername(body.Username) || !auth.ValidPassword(body.Password) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("username 3-32 chars starting with a letter; password at least 8 chars"))
		return
	}
	hash, err := auth.Hash(body.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.Store.CreatePanelUser(body.Username, hash, "admin"); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	setup.Clear(s.Cfg.DataDir)
	s.Auth.OK(ip)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

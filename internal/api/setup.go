package api

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/auth"
	"github.com/siroc-dev/siroc/internal/setup"
	"github.com/siroc-dev/siroc/internal/software"
	"github.com/siroc-dev/siroc/internal/validate"
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
		"stack":      software.BaseStack(),
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
		LEEmail  string `json:"leEmail"`
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
	leEmail := strings.TrimSpace(body.LEEmail)
	if leEmail == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("Let's Encrypt account email is required"))
		return
	}
	if err := validate.Email(leEmail); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid Let's Encrypt email"))
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
	_ = s.Store.SetSetting("le_email", leEmail)
	_ = s.Store.SetSetting("le_server", "production")
	le, leErr := s.registerLEAccount()
	setup.Clear(s.Cfg.DataDir)
	s.Auth.OK(ip)
	resp := map[string]any{"ok": true, "leRegistered": false}
	if leErr == nil && le != nil {
		resp["leRegistered"] = le.Registered
		resp["leMessage"] = le.Message
	} else if leErr != nil {
		resp["leMessage"] = leErr.Error()
		log.Printf("setup: Let's Encrypt account: %v", leErr)
	}
	go s.enqueueBaseStack()
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) enqueueBaseStack() {
	for i := 0; i < 30; i++ {
		if _, err := s.Agent.Health(); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	for _, it := range software.BaseStack() {
		if s.baseStackPresent(it.Name, it.Version) && !s.baseStackFailed(it.Name, it.Version) {
			continue
		}
		if _, err := s.enqueueInstall(it.Name, it.Version); err != nil {
			if strings.Contains(err.Error(), "already queued") || strings.Contains(err.Error(), "already installing") {
				continue
			}
			log.Printf("base stack %s %s: %v", it.Name, it.Version, err)
			continue
		}
		log.Printf("base stack queued %s %s", it.Name, it.Version)
	}
}

func (s *Server) baseStackPresent(name, version string) bool {
	switch name {
	case "php":
		list, err := s.Agent.PHPVersions()
		if err != nil {
			return false
		}
		want := version
		if want == "" {
			want = software.BasePHPVersion
		}
		for _, v := range list {
			if v == want {
				return true
			}
		}
		return false
	default:
		pkgs, err := s.Agent.Packages()
		if err != nil {
			return false
		}
		for _, p := range pkgs {
			if p.Name == name && p.Installed {
				return true
			}
		}
		return false
	}
}

func (s *Server) baseStackFailed(name, version string) bool {
	st, err := s.Store.LastInstallStatus(name, version)
	return err == nil && st == "error"
}

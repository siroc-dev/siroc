package api

import (
	"fmt"
	"log"
	"net/http"
	"strings"

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
	if err := s.Store.SetSetting(stackPromptSetting, "pending"); err != nil {
		log.Printf("setup: stack prompt: %v", err)
	}
	writeJSON(w, http.StatusOK, resp)
}

const stackPromptSetting = "stack_prompt"

func (s *Server) stackPromptPending() bool {
	v, err := s.Store.Setting(stackPromptSetting)
	return err == nil && v == "pending"
}

func (s *Server) acceptStack(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(currentUser(r)) {
		writeErr(w, http.StatusForbidden, fmt.Errorf("admin only"))
		return
	}
	var body struct {
		Skip bool   `json:"skip"`
		DB   string `json:"db"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !body.Skip {
		pkgs, err := software.RecommendedInstall(body.DB)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		for _, it := range pkgs {
			if _, err := s.enqueueInstall(it.Name, it.Version); err != nil {
				msg := err.Error()
				if strings.Contains(msg, "already queued") || strings.Contains(msg, "already installing") {
					continue
				}
				writeErr(w, http.StatusBadRequest, err)
				return
			}
		}
	}
	if err := s.Store.SetSetting(stackPromptSetting, "done"); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

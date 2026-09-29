package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/secret"
	"github.com/siroc-dev/siroc/internal/store"
	"github.com/siroc-dev/siroc/internal/validate"
)

func (s *Server) siteFromID(w http.ResponseWriter, r *http.Request) (*store.Site, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid id"))
		return nil, false
	}
	st, err := s.Store.GetSite(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, fmt.Errorf("site not found"))
		return nil, false
	}
	if !s.allowAccount(w, r, st.Username) {
		return nil, false
	}
	return st, true
}

func (s *Server) getSiteGit(w http.ResponseWriter, r *http.Request) {
	st, ok := s.siteFromID(w, r)
	if !ok {
		return
	}
	g, err := s.ensureGit(st)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, s.gitView(r, st, g))
}

func (s *Server) putSiteGit(w http.ResponseWriter, r *http.Request) {
	st, ok := s.siteFromID(w, r)
	if !ok {
		return
	}
	var body struct {
		Repo    string `json:"repo"`
		Branch  string `json:"branch"`
		Path    string `json:"path"`
		Command string `json:"command"`
	}
	if !decode(w, r, &body) {
		return
	}
	repo, err := validate.GitRepo(body.Repo)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	branch, err := validate.GitBranch(body.Branch)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	command, err := validate.GitCommand(body.Command)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	path := strings.TrimSpace(body.Path)
	if path == "" {
		path = validate.RelHome(s.Cfg.HomeRoot, st.Username, st.DocRoot)
	}
	if _, err := validate.AccountPath(s.Cfg.HomeRoot, st.Username, path, ""); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	g, err := s.ensureGit(st)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	g.Repo = repo
	g.Branch = branch
	g.Path = path
	g.Command = command
	if err := s.Store.UpsertSiteGit(g); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, s.gitView(r, st, g))
}

func (s *Server) rotateSiteGitToken(w http.ResponseWriter, r *http.Request) {
	st, ok := s.siteFromID(w, r)
	if !ok {
		return
	}
	g, err := s.ensureGit(st)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	token, err := secret.NewKey()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	g.Token = token
	if err := s.Store.UpsertSiteGit(g); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, s.gitView(r, st, g))
}

func (s *Server) deploySiteGit(w http.ResponseWriter, r *http.Request) {
	st, ok := s.siteFromID(w, r)
	if !ok {
		return
	}
	g, err := s.Store.GetSiteGit(st.ID)
	if err != nil || strings.TrimSpace(g.Repo) == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("save a git repo first"))
		return
	}
	if !s.beginGitDeploy(st.ID) {
		writeErr(w, http.StatusConflict, fmt.Errorf("deploy already running"))
		return
	}
	defer s.endGitDeploy(st.ID)
	s.runGitDeploy(w, st, g)
}

func (s *Server) gitWebhook(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(chi.URLParam(r, "token"))
	g, err := s.Store.GetSiteGitByToken(token)
	if err != nil {
		writeErr(w, http.StatusNotFound, fmt.Errorf("unknown webhook"))
		return
	}
	st, err := s.Store.GetSite(g.SiteID)
	if err != nil {
		writeErr(w, http.StatusNotFound, fmt.Errorf("site not found"))
		return
	}
	if strings.TrimSpace(g.Repo) == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("git repo is not configured"))
		return
	}
	if !s.beginGitDeploy(st.ID) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Deploy already running"})
		return
	}
	stCopy := *st
	gCopy := *g
	go func() {
		defer s.endGitDeploy(stCopy.ID)
		_, _, _ = s.execGitDeploy(&stCopy, &gCopy)
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "message": "Deploy started"})
}

func (s *Server) beginGitDeploy(siteID int64) bool {
	_, loaded := s.gitBusy.LoadOrStore(siteID, time.Now())
	return !loaded
}

func (s *Server) endGitDeploy(siteID int64) {
	s.gitBusy.Delete(siteID)
}

func (s *Server) runGitDeploy(w http.ResponseWriter, st *store.Site, g *store.SiteGit) {
	log, ok, msg := s.execGitDeploy(st, g)
	if !ok {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("%s", msg))
		return
	}
	g.LastOK = true
	g.LastLog = log
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg, "log": log})
}

func (s *Server) execGitDeploy(st *store.Site, g *store.SiteGit) (log string, ok bool, msg string) {
	path := strings.TrimSpace(g.Path)
	if path == "" {
		path = validate.RelHome(s.Cfg.HomeRoot, st.Username, st.DocRoot)
	}
	out, err := s.Agent.GitDeploy(rpc.GitDeployReq{
		Username: st.Username,
		Path:     path,
		Repo:     g.Repo,
		Branch:   g.Branch,
		Command:  g.Command,
	})
	ok = err == nil
	msg = "Deployed"
	if out != nil {
		log = out.Log
		if out.Message != "" {
			msg = out.Message
		}
		if !out.OK && err == nil {
			ok = false
		}
	}
	if err != nil {
		ok = false
		if log == "" {
			log = err.Error()
		}
		msg = err.Error()
	}
	_ = s.Store.UpdateSiteGitResult(st.ID, ok, log)
	return log, ok, msg
}

func (s *Server) ensureGit(st *store.Site) (*store.SiteGit, error) {
	if g, err := s.Store.GetSiteGit(st.ID); err == nil {
		return g, nil
	}
	token, err := secret.NewKey()
	if err != nil {
		return nil, err
	}
	g := &store.SiteGit{
		SiteID:  st.ID,
		Branch:  "main",
		Path:    validate.RelHome(s.Cfg.HomeRoot, st.Username, st.DocRoot),
		Token:   token,
		Command: "",
	}
	if err := s.Store.UpsertSiteGit(g); err != nil {
		return nil, err
	}
	return g, nil
}

func (s *Server) gitView(r *http.Request, st *store.Site, g *store.SiteGit) map[string]any {
	key, _ := s.Agent.GitKey(st.Username)
	pub, keyPath, fp := "", "", ""
	if key != nil {
		pub = key.PublicKey
		keyPath = key.KeyPath
		fp = key.Fingerprint
	}
	scheme := "https"
	if r.TLS == nil && s.Cfg.DisableTLS {
		scheme = "http"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto == "http" || proto == "https" {
		scheme = proto
	}
	hook := fmt.Sprintf("%s://%s/api/hooks/git/%s", scheme, r.Host, g.Token)
	return map[string]any{
		"repo":        g.Repo,
		"branch":      g.Branch,
		"path":        g.Path,
		"command":     g.Command,
		"token":       g.Token,
		"webhookUrl":  hook,
		"publicKey":   pub,
		"keyPath":     keyPath,
		"fingerprint": fp,
		"lastOk":      g.LastOK,
		"lastLog":     g.LastLog,
		"lastAt":      g.LastAt,
		"laravelHint": validate.LaravelDeployCommand,
		"npmHint":     validate.NPMDeployCommand,
	}
}

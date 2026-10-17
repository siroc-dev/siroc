package api

import (
	"fmt"
	"net/http"
	"regexp"
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

var gitUserInfoRe = regexp.MustCompile(`(?i)(https?://)[^/\s:@]+:[^/\s@]+@`)

func (s *Server) gitBranches(w http.ResponseWriter, r *http.Request) {
	st, ok := s.siteFromID(w, r)
	if !ok {
		return
	}
	var body struct {
		Repo string `json:"repo"`
	}
	if !decode(w, r, &body) {
		return
	}
	raw := strings.TrimSpace(body.Repo)
	if raw == "" {
		if g, err := s.Store.GetSiteGit(st.ID); err == nil {
			raw = g.Repo
		}
	}
	repo, urlBranch, err := validate.ParseGitRemote(raw)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	out, err := s.Agent.GitBranches(st.Username, repo)
	view := map[string]any{"ok": false, "repo": repo}
	if urlBranch != "" {
		view["branch"] = urlBranch
	}
	if err != nil || out == nil || !out.OK {
		msg := "could not list branches"
		if err != nil {
			msg = err.Error()
		} else if out != nil && out.Message != "" {
			msg = out.Message
		}
		msg = redactGitURL(msg)
		view["message"] = msg
		view["needsKey"] = validate.GitNeedsDeployKey(msg)
		writeJSON(w, http.StatusOK, view)
		return
	}
	view["ok"] = true
	view["branches"] = out.Branches
	view["head"] = out.Head
	writeJSON(w, http.StatusOK, view)
}

func redactGitURL(s string) string {
	s = gitUserInfoRe.ReplaceAllString(s, "${1}")
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		s = s[len(s)-500:]
	}
	return s
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
	repo, urlBranch, err := validate.ParseGitRemote(body.Repo)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	branch, err := validate.GitBranch(body.Branch)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if urlBranch != "" {
		branch = urlBranch
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
	if _, err := validate.DocRoot(s.Cfg.HomeRoot, st.Username, path, ""); err != nil {
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
	useHead := urlBranch == "" && (strings.HasPrefix(repo, "git@") || strings.HasPrefix(repo, "ssh://")) && (strings.TrimSpace(body.Branch) == "" || branch == "main")
	if !s.beginGitDeploy(st.ID) {
		writeErr(w, http.StatusConflict, fmt.Errorf("deploy already running"))
		return
	}
	defer s.endGitDeploy(st.ID)
	s.execGitDeploy(st, g, useHead)
	if fresh, err := s.Store.GetSiteGit(st.ID); err == nil {
		g = fresh
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
		_, _, _, _ = s.execGitDeploy(&stCopy, &gCopy, false)
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
	log, commit, ok, msg := s.execGitDeploy(st, g, false)
	if !ok {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("%s", msg))
		return
	}
	g.LastOK = true
	g.LastLog = log
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg, "log": log, "commit": commit})
}

func (s *Server) execGitDeploy(st *store.Site, g *store.SiteGit, useHead bool) (log string, commit *rpc.GitCommit, ok bool, msg string) {
	path := strings.TrimSpace(g.Path)
	if path == "" {
		path = validate.RelHome(s.Cfg.HomeRoot, st.Username, st.DocRoot)
	}
	out, err := s.Agent.GitDeploy(rpc.GitDeployReq{
		Username:      st.Username,
		Path:          path,
		Repo:          g.Repo,
		Branch:        g.Branch,
		Command:       g.Command,
		UseRemoteHead: useHead,
	})
	ok = err == nil
	msg = "Deployed"
	if out != nil {
		log = out.Log
		commit = out.Commit
		if out.Branch != "" {
			g.Branch = out.Branch
			_ = s.Store.UpdateSiteGitBranch(st.ID, out.Branch)
		}
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
	return log, commit, ok, msg
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
	path := strings.TrimSpace(g.Path)
	if path == "" {
		path = validate.RelHome(s.Cfg.HomeRoot, st.Username, st.DocRoot)
	}
	var commit *rpc.GitCommit
	if head, err := s.Agent.GitHead(st.Username, path); err == nil && head != nil && head.Subject != "" {
		commit = head
	}
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
		"needsKey":    !g.LastOK && validate.GitNeedsDeployKey(g.LastLog),
		"lastLog":     g.LastLog,
		"lastAt":      g.LastAt,
		"commit":      commit,
		"laravelHint": validate.LaravelDeployCommand,
		"npmHint":     validate.NPMDeployCommand,
	}
}

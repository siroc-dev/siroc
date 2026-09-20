package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/store"
	"github.com/siroc-dev/siroc/internal/validate"
)

func (s *Server) phpExtensions(w http.ResponseWriter, r *http.Request) {
	ver := r.URL.Query().Get("version")
	if ver == "" {
		ver = "8.3"
	}
	if err := validate.PHPVersion(ver); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	out, err := s.Agent.PHPExtensions(ver)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) installPHPExt(w http.ResponseWriter, r *http.Request) {
	var body rpc.PHPExtInstallReq
	if !decode(w, r, &body) {
		return
	}
	if err := validate.PHPVersion(body.Version); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := validate.PHPExtName(body.Name); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	source := body.Source
	if source == "" {
		source = "auto"
	}
	job, err := s.enqueueInstall("php-ext", rpc.PHPExtJobSpec(body.Version, body.Name, source))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job": job, "queued": 1})
}

func (s *Server) getAccountPHP(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	acc, ok := s.accountOrErr(w, r, username)
	if !ok {
		return
	}
	settings := s.accountFPM(acc)
	if acc.PHPFpmJSON == "" {
		phps, err := s.Agent.PHPVersions()
		if err == nil {
			settings.Extensions = map[string][]string{}
			for _, ver := range phps {
				exts, err := s.Agent.PHPExtensions(ver)
				if err != nil {
					continue
				}
				var names []string
				for _, e := range exts.Extensions {
					if e.Installed && e.Source != "system" {
						names = append(names, e.Name)
					}
				}
				settings.Extensions[ver] = names
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"username": acc.Username,
		"settings": settings,
		"custom":   acc.PHPFpmJSON != "",
	})
}

func (s *Server) setAccountPHP(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	acc, ok := s.accountOrErr(w, r, username)
	if !ok {
		return
	}
	var body rpc.PHPFPMSettings
	if !decode(w, r, &body) {
		return
	}
	settings := rpc.MergePHPFPM(body)
	settings.OpenBasedir = true
	if err := validate.PHPFPM(settings.PM, settings.MaxChildren, settings.StartServers, settings.MinSpare, settings.MaxSpare, settings.MaxRequests, settings.MaxExecutionTime, settings.MaxInputTime); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	sites, err := s.Store.ListSites()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	var reqSites []rpc.SiteWriteReq
	for _, st := range sites {
		if st.Username != acc.Username {
			continue
		}
		req := s.siteWriteReq(st, st.PHPVersion, st.Enabled, st.Aliases, st.SSL, st.SSLKind, st.Rewrite)
		req.FPM = &settings
		reqSites = append(reqSites, req)
	}
	if err := s.Agent.PHPUserApply(rpc.PHPUserApplyReq{Username: acc.Username, Settings: settings, Sites: reqSites}); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.Store.UpdateAccountPHP(acc.Username, string(raw)); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "settings": settings})
}

func (s *Server) accountFPM(acc *store.Account) rpc.PHPFPMSettings {
	if acc == nil || acc.PHPFpmJSON == "" {
		return rpc.DefaultPHPFPM()
	}
	var in rpc.PHPFPMSettings
	if err := json.Unmarshal([]byte(acc.PHPFpmJSON), &in); err != nil {
		return rpc.DefaultPHPFPM()
	}
	return rpc.MergePHPFPM(in)
}

func (s *Server) accountFPMPtr(acc *store.Account) *rpc.PHPFPMSettings {
	if acc == nil || acc.PHPFpmJSON == "" {
		return nil
	}
	st := s.accountFPM(acc)
	return &st
}

func (s *Server) accountFPMPtrByName(username string) *rpc.PHPFPMSettings {
	acc, err := s.Store.GetAccountByName(username)
	if err != nil {
		return nil
	}
	return s.accountFPMPtr(acc)
}

func (s *Server) requirePHPInstalled(ver string) error {
	if err := validate.PHPVersion(ver); err != nil {
		return err
	}
	list, err := s.Agent.PHPVersions()
	if err != nil {
		return err
	}
	for _, v := range list {
		if v == ver {
			return nil
		}
	}
	return fmt.Errorf("PHP %s is not installed", ver)
}

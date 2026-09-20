package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/siroc-dev/siroc/internal/store"
)

func (s *Server) setAccountWAF(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	acc, ok := s.accountOrErr(w, r, username)
	if !ok {
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if !decode(w, r, &body) {
		return
	}
	acc.WAFEnabled = body.Enabled
	if err := s.rewriteAccountSites(*acc); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.Store.UpdateAccountWAF(acc.Username, body.Enabled); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, acc)
}

func (s *Server) setSiteWAF(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid id"))
		return
	}
	st, err := s.Store.GetSite(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, fmt.Errorf("site not found"))
		return
	}
	if !s.allowAccount(w, r, st.Username) {
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if !decode(w, r, &body) {
		return
	}
	st.WAFEnabled = body.Enabled
	req := s.siteWriteReq(*st, st.PHPVersion, st.Enabled, st.Aliases, st.SSL, st.SSLKind, st.Rewrite)
	if err := s.Agent.SiteWrite(req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.Store.UpdateSiteWAF(st.ID, body.Enabled); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) rewriteAccountSites(acc store.Account) error {
	sites, err := s.Store.ListSites()
	if err != nil {
		return err
	}
	for _, st := range sites {
		if st.Username != acc.Username {
			continue
		}
		req := s.siteWriteReq(st, st.PHPVersion, st.Enabled, st.Aliases, st.SSL, st.SSLKind, st.Rewrite)
		req.WAF = store.EffectiveWAF(acc.WAFEnabled, st.WAFEnabled)
		if err := s.Agent.SiteWrite(req); err != nil {
			return fmt.Errorf("%s: %w", st.Domain, err)
		}
	}
	return nil
}

func (s *Server) siteWAF(st store.Site) bool {
	acc, err := s.Store.GetAccountByName(st.Username)
	if err != nil {
		return st.WAFEnabled
	}
	return store.EffectiveWAF(acc.WAFEnabled, st.WAFEnabled)
}

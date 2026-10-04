package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/siroc-dev/siroc/internal/rpc"
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

func (s *Server) siteWAFRules(w http.ResponseWriter, r *http.Request) {
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
	if r.Method == http.MethodPut {
		var body struct {
			DisabledIDs []int `json:"disabledIds"`
		}
		if !decode(w, r, &body) {
			return
		}
		ids, err := store.NormalizeWAFRuleIDs(body.DisabledIDs)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		st.WAFDisabledIDs = ids
		req := s.siteWriteReq(*st, st.PHPVersion, st.Enabled, st.Aliases, st.SSL, st.SSLKind, st.Rewrite)
		if err := s.Agent.SiteWrite(req); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if err := s.Store.UpdateSiteWAFRules(st.ID, ids); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		st, _ = s.Store.GetSite(id)
		writeJSON(w, http.StatusOK, st)
		return
	}
	out, err := s.Agent.WAFRules(r.URL.Query().Get("q"), r.URL.Query().Get("pack"))
	if err != nil || out == nil {
		out = &rpc.WAFRulesResp{Rules: []rpc.WAFRule{}, Total: 0}
	}
	off := map[int]bool{}
	for _, id := range st.WAFDisabledIDs {
		off[id] = true
	}
	for i := range out.Rules {
		out.Rules[i].Disabled = off[out.Rules[i].ID]
	}
	writeJSON(w, http.StatusOK, struct {
		Rules       []rpc.WAFRule `json:"rules"`
		Total       int           `json:"total"`
		DisabledIDs []int         `json:"disabledIds"`
	}{Rules: out.Rules, Total: out.Total, DisabledIDs: st.WAFDisabledIDs})
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

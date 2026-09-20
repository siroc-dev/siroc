package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/store"
)

func (s *Server) logStatus(w http.ResponseWriter, _ *http.Request) {
	st, err := s.Agent.LogStatus()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if st.RetentionDays == 0 {
		st.RetentionDays = 90
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) setLogRetention(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Days int `json:"days"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Days < 1 || body.Days > 3650 {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("keep logs between 1 and 3650 days"))
		return
	}
	st, err := s.Agent.LogRetention(body.Days)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = s.Store.SetSetting("log_retention_days", strconv.Itoa(body.Days))
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) refreshCloudflare(w http.ResponseWriter, _ *http.Request) {
	st, err := s.Agent.CloudflareRefresh()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) siteStatsInfo(w http.ResponseWriter, r *http.Request) {
	st, ok := s.siteForStats(w, r)
	if !ok {
		return
	}
	refresh := r.URL.Query().Get("refresh") == "1"
	if r.Method == http.MethodPost {
		var body struct {
			Refresh bool `json:"refresh"`
		}
		if r.ContentLength != 0 && !decode(w, r, &body) {
			return
		}
		refresh = body.Refresh
	}
	info, err := s.Agent.GoAccessInfo(st.Domain, refresh)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) siteStatsHTML(w http.ResponseWriter, r *http.Request) {
	st, ok := s.siteForStats(w, r)
	if !ok {
		return
	}
	b, err := s.Agent.GoAccessHTML(st.Domain, r.URL.Query().Get("refresh") == "1")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

func (s *Server) siteLogs(w http.ResponseWriter, r *http.Request) {
	st, ok := s.siteForStats(w, r)
	if !ok {
		return
	}
	out, err := s.Agent.SiteLogs(rpc.SiteLogReq{
		Username: st.Username,
		Domain:   st.Domain,
		DocRoot:  st.DocRoot,
		ID:       r.URL.Query().Get("id"),
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if out != nil {
		for i := range out.Files {
			out.Files[i].Path = ""
		}
		if out.Current != nil {
			out.Current.Path = ""
		}
		if out.Entries == nil {
			out.Entries = []rpc.SiteLogEntry{}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) siteForStats(w http.ResponseWriter, r *http.Request) (*store.Site, bool) {
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

package api

import (
	"fmt"
	"net/http"

	"github.com/siroc-dev/siroc/internal/validate"
)

func (s *Server) getDefaultSite(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(currentUser(r)) {
		writeErr(w, http.StatusForbidden, fmt.Errorf("admin only"))
		return
	}
	domain, err := s.Store.Setting("default_site")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"domain": domain})
}

func (s *Server) setDefaultSite(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(currentUser(r)) {
		writeErr(w, http.StatusForbidden, fmt.Errorf("admin only"))
		return
	}
	var body struct {
		Domain string `json:"domain"`
	}
	if !decode(w, r, &body) {
		return
	}
	domain := ""
	if body.Domain != "" {
		var err error
		domain, err = validate.ASCIIHost(body.Domain)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if _, err := s.Store.GetSiteByDomain(domain); err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("website not found"))
			return
		}
	}
	if err := s.Agent.SetDefaultSite(domain); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.Store.SetSetting("default_site", domain); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"domain": domain})
}

func (s *Server) clearDefaultSite(domain string) error {
	current, err := s.Store.Setting("default_site")
	if err != nil {
		return err
	}
	if current == "" || current != domain {
		return nil
	}
	if err := s.Agent.SetDefaultSite(""); err != nil {
		return err
	}
	return s.Store.SetSetting("default_site", "")
}

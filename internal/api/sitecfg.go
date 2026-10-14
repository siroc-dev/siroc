package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/store"
	"github.com/siroc-dev/siroc/internal/validate"
)

func (s *Server) siteConfig(w http.ResponseWriter, r *http.Request) {
	st, ok := s.siteForConfig(w, r)
	if !ok {
		return
	}
	out, err := s.Agent.SiteConfigRead(st.Domain)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	out.Custom = strings.TrimSpace(st.NginxConf) != "" || strings.TrimSpace(st.ApacheConf) != ""
	out.Nginx = validate.FormatNginx(out.Nginx)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) testSiteConfig(w http.ResponseWriter, r *http.Request) {
	st, ok := s.siteForConfig(w, r)
	if !ok {
		return
	}
	body, ok := s.configBody(w, r)
	if !ok {
		return
	}
	body.Domain = st.Domain
	body.Enabled = st.Enabled
	out, err := s.Agent.SiteConfigTest(body)
	s.writeConfigResult(w, out, err)
}

func (s *Server) saveSiteConfig(w http.ResponseWriter, r *http.Request) {
	st, ok := s.siteForConfig(w, r)
	if !ok {
		return
	}
	body, ok := s.configBody(w, r)
	if !ok {
		return
	}
	body.Domain = st.Domain
	body.Enabled = st.Enabled
	out, err := s.Agent.SiteConfigSave(body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if out == nil || !out.OK {
		msg := "config test failed"
		log := ""
		if out != nil {
			log = out.Log
			if out.Message != "" {
				msg = out.Message
			}
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg, "log": log})
		return
	}
	apache := body.Apache
	if strings.TrimSpace(apache) == "" {
		if cur, err := s.Agent.SiteConfigRead(st.Domain); err == nil && cur != nil {
			apache = cur.Apache
		}
	}
	if err := s.Store.UpdateSiteConfig(st.ID, body.Nginx, apache); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out.Nginx = body.Nginx
	out.Apache = apache
	out.Custom = true
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) resetSiteConfig(w http.ResponseWriter, r *http.Request) {
	st, ok := s.siteForConfig(w, r)
	if !ok {
		return
	}
	req := s.siteWriteReq(*st, st.PHPVersion, st.Enabled, st.Aliases, st.SSL, st.SSLKind, st.Rewrite)
	req.NginxConf = ""
	req.ApacheConf = ""
	if err := s.Agent.SiteWrite(req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.Store.UpdateSiteConfig(st.ID, "", ""); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) siteForConfig(w http.ResponseWriter, r *http.Request) (*store.Site, bool) {
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

func (s *Server) configBody(w http.ResponseWriter, r *http.Request) (rpc.SiteConfigReq, bool) {
	var body rpc.SiteConfigReq
	if !decode(w, r, &body) {
		return body, false
	}
	body.Nginx = validate.FormatNginx(body.Nginx)
	body.Apache = strings.ReplaceAll(body.Apache, "\r\n", "\n")
	if err := validate.SiteConfigText("nginx", body.Nginx); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return body, false
	}
	if strings.TrimSpace(body.Nginx) == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("nginx config is empty"))
		return body, false
	}
	if err := validate.SiteConfigText("apache", body.Apache); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return body, false
	}
	return body, true
}

func (s *Server) writeConfigResult(w http.ResponseWriter, out *rpc.SiteConfigResp, err error) {
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if out == nil || !out.OK {
		msg := "config test failed"
		log := ""
		if out != nil {
			log = out.Log
			if out.Message != "" {
				msg = out.Message
			}
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg, "log": log})
		return
	}
	writeJSON(w, http.StatusOK, out)
}

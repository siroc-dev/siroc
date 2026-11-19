package api

import (
	"net/http"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func (s *Server) panelCertReq() rpc.PanelCertReq {
	return rpc.PanelCertReq{
		DataDir:  s.Cfg.DataDir,
		CertFile: s.Cfg.TLSCert,
		KeyFile:  s.Cfg.TLSKey,
	}
}

func (s *Server) panelCertificate(w http.ResponseWriter, _ *http.Request) {
	out, err := s.Agent.PanelCertStatus(s.panelCertReq())
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) issuePanelCertificate(w http.ResponseWriter, _ *http.Request) {
	out, err := s.Agent.IssuePanelCert(s.panelCertReq())
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

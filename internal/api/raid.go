package api

import (
	"net/http"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func (s *Server) raidStatus(w http.ResponseWriter, _ *http.Request) {
	out, err := s.Agent.RAID()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) raidInstall(w http.ResponseWriter, _ *http.Request) {
	out, err := s.Agent.RAIDInstall()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) raidCreate(w http.ResponseWriter, r *http.Request) {
	var body rpc.RAIDCreateReq
	if !decode(w, r, &body) {
		return
	}
	out, err := s.Agent.RAIDCreate(body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

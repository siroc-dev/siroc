package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func (s *Server) listHosts(w http.ResponseWriter, _ *http.Request) {
	out, err := s.Agent.Hosts()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createHost(w http.ResponseWriter, r *http.Request) {
	var body rpc.HostsReq
	if !decode(w, r, &body) {
		return
	}
	out, err := s.Agent.HostCreate(body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) updateHost(w http.ResponseWriter, r *http.Request) {
	var body rpc.HostsReq
	if !decode(w, r, &body) {
		return
	}
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	out, err := s.Agent.HostUpdate(id, body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) deleteHost(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	out, err := s.Agent.HostDelete(id)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

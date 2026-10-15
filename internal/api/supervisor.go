package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func (s *Server) supervisorStatus(w http.ResponseWriter, _ *http.Request) {
	out, err := s.Agent.Supervisor()
	finishSupervisor(w, out, err)
}

func (s *Server) supervisorInstall(w http.ResponseWriter, _ *http.Request) {
	out, err := s.Agent.SupervisorInstall()
	finishSupervisor(w, out, err)
}

func (s *Server) supervisorService(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action string `json:"action"`
	}
	if !decode(w, r, &body) {
		return
	}
	out, err := s.Agent.SupervisorService(body.Action)
	finishSupervisor(w, out, err)
}

func (s *Server) supervisorReload(w http.ResponseWriter, _ *http.Request) {
	out, err := s.Agent.SupervisorReload()
	finishSupervisor(w, out, err)
}

func (s *Server) supervisorCreate(w http.ResponseWriter, r *http.Request) {
	var body rpc.SupervisorProgramReq
	if !decode(w, r, &body) {
		return
	}
	out, err := s.Agent.SupervisorCreate(body)
	finishSupervisor(w, out, err)
}

func (s *Server) supervisorUpdate(w http.ResponseWriter, r *http.Request) {
	var body rpc.SupervisorProgramReq
	if !decode(w, r, &body) {
		return
	}
	out, err := s.Agent.SupervisorUpdate(chi.URLParam(r, "name"), body)
	finishSupervisor(w, out, err)
}

func (s *Server) supervisorDelete(w http.ResponseWriter, r *http.Request) {
	out, err := s.Agent.SupervisorDelete(chi.URLParam(r, "name"))
	finishSupervisor(w, out, err)
}

func (s *Server) supervisorCtl(w http.ResponseWriter, r *http.Request) {
	out, err := s.Agent.SupervisorCtl(chi.URLParam(r, "name"), chi.URLParam(r, "action"))
	finishSupervisor(w, out, err)
}

func (s *Server) supervisorLog(w http.ResponseWriter, r *http.Request) {
	text, err := s.Agent.SupervisorLog(chi.URLParam(r, "name"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"text": text})
}

func finishSupervisor(w http.ResponseWriter, out *rpc.SupervisorStatus, err error) {
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

package api

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func (s *Server) rcloneStatus(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(currentUser(r)) {
		writeErr(w, http.StatusForbidden, fmt.Errorf("admin only"))
		return
	}
	out, err := s.Agent.RcloneStatus()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) rcloneCreate(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(currentUser(r)) {
		writeErr(w, http.StatusForbidden, fmt.Errorf("admin only"))
		return
	}
	var body rpc.RcloneRemoteReq
	if !decode(w, r, &body) {
		return
	}
	if err := s.Agent.RcloneCreate(body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	out, err := s.Agent.RcloneStatus()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) rcloneDelete(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(currentUser(r)) {
		writeErr(w, http.StatusForbidden, fmt.Errorf("admin only"))
		return
	}
	if err := s.Agent.RcloneDelete(chi.URLParam(r, "name")); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	out, err := s.Agent.RcloneStatus()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) rcloneRun(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(currentUser(r)) {
		writeErr(w, http.StatusForbidden, fmt.Errorf("admin only"))
		return
	}
	var body rpc.RcloneRunReq
	if !decode(w, r, &body) {
		return
	}
	out, err := s.Agent.RcloneRun(body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) rcloneJob(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(currentUser(r)) {
		writeErr(w, http.StatusForbidden, fmt.Errorf("admin only"))
		return
	}
	out, err := s.Agent.RcloneJob()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func (s *Server) nfsStatus(w http.ResponseWriter, _ *http.Request) {
	out, err := s.Agent.NFS()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) nfsInstall(w http.ResponseWriter, _ *http.Request) {
	out, err := s.Agent.NFSInstall()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) nfsService(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action string `json:"action"`
	}
	if !decode(w, r, &body) {
		return
	}
	out, err := s.Agent.NFSService(body.Action)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) nfsExportCreate(w http.ResponseWriter, r *http.Request) {
	var body rpc.NFSExportReq
	if !decode(w, r, &body) {
		return
	}
	out, err := s.Agent.NFSExportCreate(body)
	finishNFS(w, out, err)
}

func (s *Server) nfsExportUpdate(w http.ResponseWriter, r *http.Request) {
	var body rpc.NFSExportReq
	if !decode(w, r, &body) {
		return
	}
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	out, err := s.Agent.NFSExportUpdate(id, body)
	finishNFS(w, out, err)
}

func (s *Server) nfsExportDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	out, err := s.Agent.NFSExportDelete(id)
	finishNFS(w, out, err)
}

func (s *Server) nfsMountCreate(w http.ResponseWriter, r *http.Request) {
	var body rpc.NFSMountReq
	if !decode(w, r, &body) {
		return
	}
	out, err := s.Agent.NFSMountCreate(body)
	finishNFS(w, out, err)
}

func (s *Server) nfsMountUpdate(w http.ResponseWriter, r *http.Request) {
	var body rpc.NFSMountReq
	if !decode(w, r, &body) {
		return
	}
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	out, err := s.Agent.NFSMountUpdate(id, body)
	finishNFS(w, out, err)
}

func (s *Server) nfsMountDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	out, err := s.Agent.NFSMountDelete(id)
	finishNFS(w, out, err)
}

func (s *Server) nfsMountAct(w http.ResponseWriter, r *http.Request) {
	action := chi.URLParam(r, "action")
	if action != "mount" && action != "unmount" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("unknown NFS action"))
		return
	}
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	out, err := s.Agent.NFSMountAct(id, action)
	finishNFS(w, out, err)
}

func finishNFS(w http.ResponseWriter, out *rpc.NFSStatus, err error) {
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

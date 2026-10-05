package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func (s *Server) osUpdateStatus(w http.ResponseWriter, _ *http.Request) {
	st, err := s.Agent.OSUpdates()
	if err != nil {
		writeJSON(w, http.StatusOK, rpc.OSUpdateStatus{
			OK:      false,
			Message: "Agent is offline. The package list is on the server.",
		})
		return
	}
	if st.Packages == nil {
		st.Packages = []rpc.OSPackage{}
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) osUpdate(w http.ResponseWriter, r *http.Request) {
	var body rpc.OSUpdateReq
	if !decode(w, r, &body) {
		return
	}
	action := strings.ToLower(strings.TrimSpace(body.Action))
	if action != "update" && action != "upgrade" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("action must be update or upgrade"))
		return
	}
	out, err := s.Agent.OSUpdate(action)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if out.Packages == nil {
		out.Packages = []rpc.OSPackage{}
	}
	writeJSON(w, http.StatusOK, out)
}

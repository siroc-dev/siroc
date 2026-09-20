package api

import (
	"net/http"
	"os"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/update"
	"github.com/siroc-dev/siroc/internal/version"
)

func (s *Server) panelUpdateStatus(w http.ResponseWriter, _ *http.Request) {
	channel, _ := s.Store.Setting("update_channel")
	if channel == "" {
		channel = update.ResolveChannel(os.Getenv("SIROC_UPDATE_URL"))
		_ = s.Store.SetSetting("update_channel", channel)
	}
	st, err := s.Agent.PanelUpdateStatus()
	if err != nil {
		writeJSON(w, http.StatusOK, rpc.PanelUpdateStatus{
			OK:      true,
			Version: version.Current(),
			Channel: channel,
			Message: "agent offline; showing panel version only",
		})
		return
	}
	if st.Channel == "" {
		st.Channel = channel
	}
	if st.Version == "" {
		st.Version = version.Current()
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) panelUpdate(w http.ResponseWriter, r *http.Request) {
	var body rpc.PanelUpdateReq
	if !decode(w, r, &body) {
		return
	}
	if ch := strings.TrimSpace(body.Channel); ch != "" {
		_ = s.Store.SetSetting("update_channel", ch)
	} else if saved, _ := s.Store.Setting("update_channel"); saved != "" {
		body.Channel = saved
	} else {
		body.Channel = update.ResolveChannel(os.Getenv("SIROC_UPDATE_URL"))
	}
	action := strings.ToLower(strings.TrimSpace(body.Action))
	if action == "" || action == "status" {
		s.panelUpdateStatus(w, r)
		return
	}
	out, err := s.Agent.PanelUpdate(body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if out.Version == "" {
		out.Version = version.Current()
	}
	writeJSON(w, http.StatusOK, out)
}

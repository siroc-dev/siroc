package api

import (
	"net/http"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func (s *Server) siteBandwidth(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListSites()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	user := ""
	if !isAdmin(currentUser(r)) {
		user = currentUser(r).Username
	}
	domains := make([]string, 0, len(list))
	for _, st := range list {
		if user != "" && st.Username != user {
			continue
		}
		domains = append(domains, st.Domain)
	}
	out, err := s.Agent.SiteBandwidth(domains)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if out.Sites == nil {
		out.Sites = []rpc.BandwidthSite{}
	}
	writeJSON(w, http.StatusOK, out)
}

package api

import (
	"fmt"
	"net/http"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func (s *Server) rebuildSites(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(currentUser(r)) {
		writeErr(w, http.StatusForbidden, fmt.Errorf("admin only"))
		return
	}
	sites, err := s.Store.ListSites()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	reqs := make([]rpc.SiteWriteReq, 0, len(sites))
	for _, st := range sites {
		reqs = append(reqs, s.siteWriteReq(st, st.PHPVersion, st.Enabled, st.Aliases, st.SSL, st.SSLKind, st.Rewrite))
	}
	out, err := s.Agent.SitesRebuild(reqs)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

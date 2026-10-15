package api

import (
	"net/http"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/siteopts"
)

func (s *Server) purgeSiteCache(w http.ResponseWriter, r *http.Request) {
	st, ok := s.siteForStats(w, r)
	if !ok {
		return
	}
	dir, err := siteopts.CacheDir(st.Options, st.Domain)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.Agent.PurgeSiteCache(rpc.CachePurgeReq{Path: dir}); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

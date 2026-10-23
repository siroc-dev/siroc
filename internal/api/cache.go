package api

import (
	"net/http"
	"strings"

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

func (s *Server) siteCacheDisk(w http.ResponseWriter, r *http.Request) {
	st, ok := s.siteForStats(w, r)
	if !ok {
		return
	}
	dir, err := siteopts.CacheDir(siteopts.Options{Cache: &siteopts.Cache{On: true, Path: strings.TrimSpace(r.URL.Query().Get("path"))}}, st.Domain)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]uint64{
		"total": siteopts.CacheDiskTotal(dir),
		"max":   siteopts.CacheDiskMax(dir),
	})
}

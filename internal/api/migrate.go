package api

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

var migrateApply sync.Mutex

func (s *Server) migrateDiscover(w http.ResponseWriter, r *http.Request) {
	var req rpc.DAConn
	if !decode(w, r, &req) {
		return
	}
	if len(req.PrivateKey) > 256*1024 {
		writeErr(w, http.StatusBadRequest, errors.New("private key file is too large"))
		return
	}
	list, err := s.Agent.DADiscover(req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) migrateStart(w http.ResponseWriter, r *http.Request) {
	var req rpc.DAStartReq
	if !decode(w, r, &req) {
		return
	}
	if len(req.PrivateKey) > 256*1024 {
		writeErr(w, http.StatusBadRequest, errors.New("private key file is too large"))
		return
	}
	job, err := s.Agent.DAStart(req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	stripMigrate(job)
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) migrateJob(w http.ResponseWriter, _ *http.Request) {
	job, err := s.applyMigrateJob()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) runMigrateApply() {
	var last string
	for {
		_, err := s.applyMigrateJob()
		if err != nil {
			msg := err.Error()
			if msg != last {
				log.Printf("migrate: %v", err)
				last = msg
			}
		} else {
			last = ""
		}
		time.Sleep(2 * time.Second)
	}
}

func (s *Server) applyMigrateJob() (*rpc.DAJob, error) {
	migrateApply.Lock()
	defer migrateApply.Unlock()
	job, err := s.Agent.DAJob()
	if err != nil {
		return nil, err
	}
	if job == nil {
		job = &rpc.DAJob{Items: []rpc.DAItem{}}
	}
	seen := map[string]bool{}
	for {
		var item *rpc.DAItem
		for i := range job.Items {
			if job.Items[i].Status == "records" && job.Items[i].Manifest != nil && !seen[job.Items[i].Domain] {
				item = &job.Items[i]
				break
			}
		}
		if item == nil {
			break
		}
		seen[item.Domain] = true
		ack := rpc.DAAck{Domain: item.Domain, OK: true}
		if msg := migrateConflict(s, item); msg != "" {
			ack.OK = false
			ack.Message = msg
		} else if err := s.applyBackupManifest(item.Manifest); err != nil {
			ack.OK = false
			ack.Message = err.Error()
		}
		next, err := s.Agent.DAAck(ack)
		if err != nil {
			return nil, err
		}
		job = next
	}
	stripMigrate(job)
	return job, nil
}

func migrateConflict(s *Server, item *rpc.DAItem) string {
	if item.Manifest == nil {
		return "backup has none of the selected domains"
	}
	username := strings.TrimSpace(item.Manifest.Username)
	for _, st := range item.Manifest.Sites {
		existing, err := s.Store.GetSiteByDomain(st.Domain)
		if err != nil || existing.Username == username {
			continue
		}
		return st.Domain + " already belongs to " + existing.Username
	}
	return ""
}

func stripMigrate(job *rpc.DAJob) {
	if job == nil {
		return
	}
	if job.Items == nil {
		job.Items = []rpc.DAItem{}
	}
	for i := range job.Items {
		job.Items[i].Manifest = nil
	}
}

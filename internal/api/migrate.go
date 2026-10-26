package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/siroc-dev/siroc/internal/migrate"
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
	if err := s.fillSavedMachine(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	list, err := s.Agent.DADiscover(req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) migrateSize(w http.ResponseWriter, r *http.Request) {
	var req rpc.DASizeReq
	if !decode(w, r, &req) {
		return
	}
	if len(req.PrivateKey) > 256*1024 {
		writeErr(w, http.StatusBadRequest, errors.New("private key file is too large"))
		return
	}
	if err := s.fillSavedMachine(&req.DAConn); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	list, err := s.Agent.DASize(req)
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
	if err := s.fillSavedMachine(&req.DAConn); err != nil {
		writeErr(w, http.StatusBadRequest, err)
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

func (s *Server) migrateMachine(w http.ResponseWriter, _ *http.Request) {
	m, err := migrate.LoadMachine(s.migrateMachinePath())
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if m.Mode == "" {
		m.Mode = "ssh"
	}
	if m.Port == 0 {
		m.Port = 22
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"mode":          m.Mode,
		"host":          m.Host,
		"port":          m.Port,
		"username":      m.Username,
		"insecure":      m.Insecure,
		"keyName":       m.KeyName,
		"hasPassword":   m.Password != "",
		"hasKey":        m.PrivateKey != "",
		"hasPassphrase": m.KeyPassphrase != "",
	})
}

func (s *Server) migrateMachinePath() string {
	return filepath.Join(s.Cfg.DataDir, "migrate-machine.json")
}

func (s *Server) fillSavedMachine(conn *rpc.DAConn) error {
	path := s.migrateMachinePath()
	prev, err := migrate.LoadMachine(path)
	if err != nil {
		return err
	}
	if prev.Password, err = s.openSecret(prev.Password); err != nil {
		return fmt.Errorf("saved login could not be read")
	}
	if prev.PrivateKey, err = s.openSecret(prev.PrivateKey); err != nil {
		return fmt.Errorf("saved login could not be read")
	}
	if prev.KeyPassphrase, err = s.openSecret(prev.KeyPassphrase); err != nil {
		return fmt.Errorf("saved login could not be read")
	}
	next := migrate.MergeMachine(prev, *conn)
	stored := next
	if stored.Password, err = s.sealSecret(stored.Password); err != nil {
		return err
	}
	if stored.PrivateKey, err = s.sealSecret(stored.PrivateKey); err != nil {
		return err
	}
	if stored.KeyPassphrase, err = s.sealSecret(stored.KeyPassphrase); err != nil {
		return err
	}
	if err := migrate.SaveMachine(path, stored); err != nil {
		return err
	}
	conn.Host = next.Host
	conn.Port = next.Port
	conn.Username = next.Username
	conn.Mode = next.Mode
	conn.Insecure = next.Insecure
	conn.Password = next.Password
	conn.PrivateKey = next.PrivateKey
	conn.KeyPassphrase = next.KeyPassphrase
	conn.KeyName = next.KeyName
	conn.UseSavedPassword = false
	conn.UseSavedKey = false
	conn.UseSavedPassphrase = false
	return nil
}

func (s *Server) openSecret(v string) (string, error) {
	if strings.TrimSpace(v) == "" {
		return "", nil
	}
	return s.decryptPassword(v)
}

func (s *Server) sealSecret(v string) (string, error) {
	if v == "" {
		return "", nil
	}
	return s.encryptPassword(v)
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

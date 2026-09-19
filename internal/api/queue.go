package api

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/store"
	"github.com/siroc-dev/siroc/internal/validate"
)

func (s *Server) StartInstallQueue() {
	if s.installWake == nil {
		s.installWake = make(chan struct{}, 1)
	}
	_ = s.Store.RequeueInterruptedInstalls()
	go s.runInstallQueue()
}

func (s *Server) wakeInstall() {
	if s.installWake == nil {
		return
	}
	select {
	case s.installWake <- struct{}{}:
	default:
	}
}

func (s *Server) runInstallQueue() {
	for {
		job, err := s.Store.NextQueuedInstall()
		if err != nil {
			log.Printf("install queue: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}
		if job == nil {
			select {
			case <-s.installWake:
			case <-time.After(2 * time.Second):
			}
			continue
		}
		if err := s.Store.SetInstallRunning(job.ID); err != nil {
			log.Printf("install queue start %d: %v", job.ID, err)
			continue
		}
		log.Printf("install queue: installing %s %s (job %d)", job.Name, job.Version, job.ID)
		err = s.installWithRetry(job.Name, job.Version)
		if err != nil {
			_ = s.Store.FinishInstall(job.ID, "error", err.Error())
			log.Printf("install queue: job %d failed: %v", job.ID, err)
			continue
		}
		_ = s.Store.FinishInstall(job.ID, "done", "installed")
		log.Printf("install queue: job %d done", job.ID)
	}
}

func (s *Server) installWithRetry(name, version string) error {
	var err error
	for i := 0; i < 8; i++ {
		_, err = s.Agent.Install(name, version)
		if err == nil {
			return nil
		}
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "agent unreachable") && !strings.Contains(msg, "no such file or directory") {
			return err
		}
		time.Sleep(time.Duration(i+1) * time.Second)
	}
	return err
}

func (s *Server) enqueueInstall(name, version string) (*store.InstallJob, error) {
	name = strings.TrimSpace(name)
	version = strings.TrimSpace(version)
	if name == "" {
		return nil, fmt.Errorf("package name is required")
	}
	if err := s.checkInstallAllowed(name, version); err != nil {
		return nil, err
	}
	dup, err := s.Store.HasActiveInstall(name, version)
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, fmt.Errorf("%s %s is already queued or installing", name, version)
	}
	job, err := s.Store.EnqueueInstall(name, version)
	if err != nil {
		return nil, err
	}
	s.wakeInstall()
	return job, nil
}

func (s *Server) checkInstallAllowed(name, version string) error {
	if name == "php-ext" {
		ver, ext, _, err := rpc.ParsePHPExtJob(version)
		if err != nil {
			return err
		}
		if err := validate.PHPVersion(ver); err != nil {
			return err
		}
		return validate.PHPExtName(ext)
	}
	pkgs, err := s.Agent.Packages()
	if err != nil {
		return fmt.Errorf("agent unreachable: %w", err)
	}
	var exclusive string
	found := false
	for _, p := range pkgs {
		if p.Name == name {
			found = true
			exclusive = p.ExclusiveOf
			v := strings.TrimSpace(version)
			if v == "upgrade" {
				v = ""
				exclusive = ""
			} else if strings.HasPrefix(v, "upgrade:") {
				v = strings.TrimPrefix(v, "upgrade:")
				exclusive = ""
			}
			if v != "" && len(p.Versions) > 0 {
				ok := false
				for _, cand := range p.Versions {
					if cand == v {
						ok = true
						break
					}
				}
				if !ok {
					return fmt.Errorf("unsupported version %s for %s", v, name)
				}
			}
			break
		}
	}
	if !found {
		return fmt.Errorf("unknown package %q", name)
	}
	if exclusive == "" {
		return nil
	}
	for _, p := range pkgs {
		if p.Name == exclusive && p.Installed {
			return fmt.Errorf("%s is already installed; uninstall it before installing %s", exclusive, name)
		}
	}
	active, err := s.Store.ActiveInstalls()
	if err != nil {
		return err
	}
	for _, j := range active {
		if j.Name == exclusive {
			return fmt.Errorf("%s is already in the install queue", exclusive)
		}
	}
	return nil
}

func (s *Server) installSoftware(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Items   []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"items"`
	}
	if !decode(w, r, &body) {
		return
	}
	items := body.Items
	if body.Name != "" {
		items = append([]struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}{{Name: body.Name, Version: body.Version}}, items...)
	}
	if len(items) == 0 {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("nothing to install"))
		return
	}
	var jobs []store.InstallJob
	for _, it := range items {
		job, err := s.enqueueInstall(it.Name, it.Version)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		jobs = append(jobs, *job)
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"jobs": jobs, "queued": len(jobs)})
}

func (s *Server) listInstallJobs(w http.ResponseWriter, _ *http.Request) {
	list, err := s.Store.ListInstallJobs(50)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	active, err := s.Store.ActiveInstalls()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	var current *store.InstallJob
	queued := []store.InstallJob{}
	for i := range active {
		if active[i].Status == "running" && current == nil {
			job := active[i]
			current = &job
			continue
		}
		queued = append(queued, active[i])
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"jobs":    list,
		"active":  len(active),
		"current": current,
		"queue":   queued,
	})
}

func (s *Server) cancelInstallJob(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid id"))
		return
	}
	if err := s.Store.CancelInstall(id); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) cleanInstallTemp(w http.ResponseWriter, _ *http.Request) {
	out, err := s.Agent.CleanTemp()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) cleanInstallLog(w http.ResponseWriter, _ *http.Request) {
	n, err := s.Store.ClearFinishedInstalls()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := rpc.CleanupResult{OK: true, Jobs: int(n), Message: fmt.Sprintf("Cleared %d queue logs", n)}
	if logs, err := s.Agent.CleanLogs(); err == nil && logs != nil {
		out.Freed = logs.Freed
		out.Files = logs.Files
		if logs.Freed > 0 || logs.Files > 0 {
			out.Message = fmt.Sprintf("Cleared %d queue logs · %s", n, logs.Message)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

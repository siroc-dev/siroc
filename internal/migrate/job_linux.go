//go:build linux

package migrate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/siroc-dev/siroc/internal/backup"
	"github.com/siroc-dev/siroc/internal/diskspace"
	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/validate"
)

type Runner struct {
	mu      sync.Mutex
	dataDir string
	job     rpc.DAJob
	conn    rpc.DAConn
}

var runner *Runner

func Init(dataDir string) error {
	r := &Runner{dataDir: dataDir, job: rpc.DAJob{Items: []rpc.DAItem{}}}
	err := r.load()
	if err != nil {
		r.job = rpc.DAJob{Items: []rpc.DAItem{}}
	} else {
		r.recover()
	}
	runner = r
	return err
}

func Start(req rpc.DAStartReq) (rpc.DAJob, error) {
	if runner == nil {
		return rpc.DAJob{}, fmt.Errorf("migration is not ready")
	}
	return runner.start(req)
}

func Job() rpc.DAJob {
	if runner == nil {
		return rpc.DAJob{Items: []rpc.DAItem{}}
	}
	return runner.snapshot(true)
}

func Ack(req rpc.DAAck) rpc.DAJob {
	if runner == nil {
		return rpc.DAJob{Items: []rpc.DAItem{}}
	}
	return runner.ack(req)
}

func (r *Runner) start(req rpc.DAStartReq) (rpc.DAJob, error) {
	var apiUser string
	if sshMode(req.DAConn) {
		if err := CheckSSH(req.DAConn); err != nil {
			return rpc.DAJob{}, err
		}
	} else {
		client, err := Connect(req.DAConn)
		if err != nil {
			return rpc.DAJob{}, err
		}
		apiUser = client.User
	}
	if err := validate.PHPVersion(req.PHPVersion); err != nil {
		return rpc.DAJob{}, err
	}
	if len(req.Sites) == 0 {
		return rpc.DAJob{}, fmt.Errorf("select at least one website")
	}
	if len(req.Sites) > 100 {
		return rpc.DAJob{}, fmt.Errorf("select up to 100 websites at a time")
	}
	seen := map[string]bool{}
	items := make([]rpc.DAItem, 0, len(req.Sites))
	for _, site := range req.Sites {
		domain, err := validate.ASCIIHost(site.Domain)
		if err != nil {
			return rpc.DAJob{}, err
		}
		if seen[domain] {
			return rpc.DAJob{}, fmt.Errorf("%s is selected more than once", domain)
		}
		seen[domain] = true
		owner := strings.TrimSpace(site.Owner)
		if owner == "" {
			if apiUser == "" {
				return rpc.DAJob{}, fmt.Errorf("%s has no DirectAdmin user", domain)
			}
			owner = apiUser
		}
		items = append(items, rpc.DAItem{Domain: domain, Owner: owner, Status: "queued", Message: "Waiting"})
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.job.Status == "running" {
		return rpc.DAJob{}, fmt.Errorf("a migration is already running")
	}
	r.conn = req.DAConn
	r.job = rpc.DAJob{
		Status:     "running",
		PHPVersion: req.PHPVersion,
		Items:      items,
		Message:    "Migrating websites from DirectAdmin",
	}
	r.persistLocked()
	go r.run()
	return cloneJob(r.job, false), nil
}

func (r *Runner) run() {
	defer func() {
		if rec := recover(); rec != nil {
			r.failRunning("migration stopped")
		}
	}()
	for {
		r.mu.Lock()
		idx := -1
		for i := range r.job.Items {
			if r.job.Items[i].Status == "queued" {
				idx = i
				r.job.Items[i].Status = "running"
				if sshMode(r.conn) {
					r.job.Items[i].Step = "download"
					r.job.Items[i].Percent = 10
					r.job.Items[i].Message = "Packing the website into a tar file"
				} else {
					r.job.Items[i].Step = "backup"
					r.job.Items[i].Percent = 10
					r.job.Items[i].Message = "Creating a backup on DirectAdmin"
				}
				break
			}
		}
		if idx < 0 {
			r.finishLocked()
			r.persistLocked()
			r.mu.Unlock()
			return
		}
		item := r.job.Items[idx]
		conn := r.conn
		php := r.job.PHPVersion
		r.persistLocked()
		r.mu.Unlock()

		err := r.one(conn, php, &item)
		r.mu.Lock()
		if err != nil {
			r.job.Items[idx].Status = "failed"
			r.job.Items[idx].Message = err.Error()
			r.job.Items[idx].Manifest = nil
			r.persistLocked()
			r.mu.Unlock()
			continue
		}
		r.job.Items[idx] = item
		r.persistLocked()
		r.mu.Unlock()
		r.waitAck(item.Domain)
	}
}

func (r *Runner) one(conn rpc.DAConn, php string, item *rpc.DAItem) error {
	if err := validate.LinuxUser(item.Owner); err != nil {
		return fmt.Errorf("DirectAdmin user %s cannot be created on this server: %w", item.Owner, err)
	}
	dir, err := diskspace.WritableTmp()
	if err != nil {
		return err
	}
	var dest string
	if sshMode(conn) {
		r.set(item.Domain, "running", "download", "Packing the website into a tar file", 20)
		var err error
		dest, err = PullSSH(conn, item.Owner, item.Domain, dir, func(msg string, pct int) {
			r.set(item.Domain, "running", "download", msg, pct)
		})
		if err != nil {
			return err
		}
	} else {
		client, err := Connect(conn)
		if err != nil {
			return err
		}
		progress := func(step string, pct int) {
			msg := "Creating a backup on DirectAdmin"
			if step == "download" {
				msg = "Downloading the backup"
			}
			r.set(item.Domain, "running", step, msg, pct)
		}
		dest, err = client.Pull(item.Owner, item.Domain, dir, progress)
		if err != nil {
			return err
		}
	}
	defer os.Remove(dest)
	r.set(item.Domain, "running", "restore", "Restoring files and databases", 75)
	resp, err := backup.RestoreDirectAdminDomains(dest, []string{item.Domain}, php)
	if err != nil {
		return err
	}
	if resp == nil || resp.Manifest == nil {
		return fmt.Errorf("backup has none of the selected domains")
	}
	item.Status = "records"
	item.Step = "records"
	item.Percent = 90
	item.Message = resp.Message
	item.Manifest = resp.Manifest
	return nil
}

func (r *Runner) set(domain, status, step, msg string, pct int) {
	r.mu.Lock()
	for i := range r.job.Items {
		if r.job.Items[i].Domain != domain {
			continue
		}
		if r.job.Items[i].Status == "failed" || r.job.Items[i].Status == "done" {
			break
		}
		r.job.Items[i].Status = status
		r.job.Items[i].Step = step
		r.job.Items[i].Message = msg
		if pct > r.job.Items[i].Percent {
			r.job.Items[i].Percent = pct
		}
		break
	}
	r.mu.Unlock()
}

func (r *Runner) waitAck(domain string) {
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		status := ""
		for _, it := range r.job.Items {
			if it.Domain == domain {
				status = it.Status
			}
		}
		r.mu.Unlock()
		if status == "done" || status == "failed" {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	r.mu.Lock()
	for i := range r.job.Items {
		if r.job.Items[i].Domain == domain && r.job.Items[i].Status == "records" {
			r.job.Items[i].Status = "failed"
			r.job.Items[i].Message = "the panel did not save this website"
			r.job.Items[i].Manifest = nil
		}
	}
	r.persistLocked()
	r.mu.Unlock()
}

func (r *Runner) ack(req rpc.DAAck) rpc.DAJob {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.job.Items {
		if r.job.Items[i].Domain != req.Domain || r.job.Items[i].Status != "records" {
			continue
		}
		r.job.Items[i].Manifest = nil
		if req.OK {
			r.job.Items[i].Status = "done"
			r.job.Items[i].Step = "done"
			r.job.Items[i].Percent = 100
			if strings.TrimSpace(req.Message) != "" {
				r.job.Items[i].Message = req.Message
			}
		} else {
			r.job.Items[i].Status = "failed"
			msg := strings.TrimSpace(req.Message)
			if msg == "" {
				msg = "could not save this website"
			}
			r.job.Items[i].Message = msg
		}
	}
	r.persistLocked()
	return cloneJob(r.job, true)
}

func (r *Runner) finishLocked() {
	ok := 0
	for _, it := range r.job.Items {
		if it.Status == "done" {
			ok++
		}
	}
	r.job.Status = "done"
	r.job.Message = fmt.Sprintf("%d of %d websites migrated", ok, len(r.job.Items))
	r.conn = rpc.DAConn{}
}

func (r *Runner) failRunning(msg string) {
	r.mu.Lock()
	for i := range r.job.Items {
		switch r.job.Items[i].Status {
		case "queued", "running", "records":
			r.job.Items[i].Status = "failed"
			r.job.Items[i].Message = msg
			r.job.Items[i].Manifest = nil
		}
	}
	r.finishLocked()
	r.persistLocked()
	r.mu.Unlock()
}

func (r *Runner) recover() {
	changed := false
	for i := range r.job.Items {
		switch r.job.Items[i].Status {
		case "queued", "running", "records":
			r.job.Items[i].Status = "failed"
			r.job.Items[i].Message = "agent restarted"
			r.job.Items[i].Manifest = nil
			changed = true
		}
	}
	if r.job.Status == "running" {
		ok := 0
		for _, it := range r.job.Items {
			if it.Status == "done" {
				ok++
			}
		}
		r.job.Status = "done"
		r.job.Message = fmt.Sprintf("%d of %d websites migrated", ok, len(r.job.Items))
		changed = true
	}
	if changed {
		r.persistLocked()
	}
}

func (r *Runner) persistLocked() {
	r.write(cloneJob(r.job, false))
}

func (r *Runner) snapshot(withManifest bool) rpc.DAJob {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneJob(r.job, withManifest)
}

func cloneJob(job rpc.DAJob, withManifest bool) rpc.DAJob {
	out := job
	out.Items = append([]rpc.DAItem(nil), job.Items...)
	if out.Items == nil {
		out.Items = []rpc.DAItem{}
	}
	if !withManifest {
		for i := range out.Items {
			out.Items[i].Manifest = nil
		}
	}
	return out
}

func (r *Runner) load() error {
	b, err := os.ReadFile(r.path())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var job rpc.DAJob
	if err := json.Unmarshal(b, &job); err != nil {
		return err
	}
	if job.Items == nil {
		job.Items = []rpc.DAItem{}
	}
	for i := range job.Items {
		job.Items[i].Manifest = nil
	}
	r.job = job
	return nil
}

func (r *Runner) write(job rpc.DAJob) {
	for i := range job.Items {
		job.Items[i].Manifest = nil
	}
	b, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(r.dataDir, 0750); err != nil {
		return
	}
	tmp := r.path() + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return
	}
	_ = os.Rename(tmp, r.path())
}

func (r *Runner) path() string {
	return filepath.Join(r.dataDir, "migrate-job.json")
}

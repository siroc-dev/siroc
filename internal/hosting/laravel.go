//go:build linux

package hosting

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/software"
	"github.com/siroc-dev/siroc/internal/users"
	"github.com/siroc-dev/siroc/internal/validate"
)

func laravelProgram(user, domain string) string {
	return "siroc-" + user + "-" + slug(domain) + "-queue"
}

func laravelCronPath(user, domain string) string {
	return "/etc/cron.d/cp-" + user + "-" + slug(domain)
}

func laravelSupPath(user, domain string) string {
	return "/etc/supervisor/conf.d/" + laravelProgram(user, domain) + ".conf"
}

func laravelQueueProgram(user, domain, queue string) string {
	return "siroc-" + user + "-" + slug(domain) + "-q-" + queue
}

func laravelQueueSupPath(user, domain, queue string) string {
	return "/etc/supervisor/conf.d/" + laravelQueueProgram(user, domain, queue) + ".conf"
}

func laravelScheduleLog(user, domain string) string {
	return filepath.Join("/home", user, "tmp", "schedule-"+domain+".log")
}

func laravelStatus(user, domain, root, phpVer string) *rpc.LaravelStatus {
	st := &rpc.LaravelStatus{
		HasArtisan:  fileOK(filepath.Join(root, "artisan")),
		HasComposer: fileOK(filepath.Join(root, "composer.json")),
		HasPackage:  fileOK(filepath.Join(root, "package.json")),
		EnvExists:   fileOK(filepath.Join(root, ".env")),
		QueueName:   "default",
		Scheduler:   fileOK(laravelCronPath(user, domain)),
		Workers:     1,
		Queues:      []rpc.LaravelQueueRow{},
		Processes:   []rpc.LaravelQueueProc{},
		Schedule:    []rpc.LaravelSchedule{},
	}
	rows := readLaravelQueueRows(user, domain)
	st.Queue = len(rows) > 0
	if len(rows) > 0 {
		names := make([]string, 0, len(rows))
		total := 0
		for _, r := range rows {
			names = append(names, r.Name)
			total += r.Workers
		}
		st.QueueName = strings.Join(names, ",")
		if total > 0 {
			st.Workers = total
		}
		st.Queues = attachQueueRuntime(user, domain, rows)
		seen := map[string]bool{}
		for _, q := range st.Queues {
			for _, p := range q.Processes {
				if seen[p.Name] {
					continue
				}
				seen[p.Name] = true
				st.Processes = append(st.Processes, p)
			}
		}
	}
	if st.Scheduler {
		st.ScheduleLast = fileModRFC3339(laravelScheduleLog(user, domain))
		st.Schedule = laravelScheduleList(user, root, phpVer, st.ScheduleLast)
	}
	return st
}

func laravelAction(req rpc.SiteAppReq, out *rpc.SiteAppResp) (*rpc.SiteAppResp, error) {
	root := out.AppRoot
	switch req.Action {
	case "queue":
		on := req.Queue != nil && *req.Queue
		specs, err := normalizeQueueSpecs(req.QueueName, req.Workers, req.Queues)
		if err != nil {
			return nil, err
		}
		if err := setLaravelQueue(req.Username, req.Domain, root, req.PHPVersion, on, specs); err != nil {
			return nil, err
		}
	case "scheduler":
		on := req.Scheduler != nil && *req.Scheduler
		if err := setLaravelScheduler(req.Username, req.Domain, root, req.PHPVersion, on); err != nil {
			return nil, err
		}
	case "composer":
		if err := ensureComposer(); err != nil {
			return nil, err
		}
		script, err := composerScript(req.Composer)
		if err != nil {
			return nil, err
		}
		msg, err := runAs(req.Username, root, 10*time.Minute, script)
		out.Output = msg
		if err != nil {
			return nil, err
		}
	case "npm":
		_, scripts := listNPMPkgs(req.Username, root)
		script, err := npmScript(req.NPM, scripts)
		if err != nil {
			return nil, err
		}
		script = prefixBin("npm", resolveUserBin(req.Username, "npm"), script)
		msg, err := runAs(req.Username, root, 10*time.Minute, script)
		out.Output = msg
		if err != nil {
			return nil, err
		}
	case "artisan":
		if !fileOK(filepath.Join(root, "artisan")) {
			return nil, fmt.Errorf("artisan not found in %s", root)
		}
		script, err := artisanScript(phpBin(req.PHPVersion), req.Artisan)
		if err != nil {
			return nil, err
		}
		msg, err := runAs(req.Username, root, 5*time.Minute, script)
		out.Output = msg
		if err != nil {
			if out.Output == "" {
				out.Output = err.Error()
			}
			out.Message = "artisan exited with an error"
			out.Laravel = laravelStatus(req.Username, req.Domain, root, req.PHPVersion)
			out.Laravel.ArtisanCmds = listArtisan(req.Username, root, req.PHPVersion)
			return out, nil
		}
	case "packages":
		out.Laravel = laravelStatus(req.Username, req.Domain, root, req.PHPVersion)
		fillPackages(out.Laravel, req.Username, root, req.PHPVersion)
		return out, nil
	case "pkg-search":
		kind := strings.TrimSpace(req.Name)
		q := strings.TrimSpace(req.Composer)
		if kind == "npm" {
			q = strings.TrimSpace(req.NPM)
			if q == "" {
				q = strings.TrimSpace(req.Composer)
			}
		}
		hits, err := pkgSearch(kind, q)
		if err != nil {
			return nil, err
		}
		out.PkgHits = hits
		return out, nil
	case "env-get":
		out.Laravel = laravelStatus(req.Username, req.Domain, root, req.PHPVersion)
		b, err := os.ReadFile(filepath.Join(root, ".env"))
		if err == nil {
			out.Laravel.Env = string(b)
		}
		return out, nil
	case "env-set":
		if strings.ContainsRune(req.Env, 0) {
			return nil, fmt.Errorf("invalid .env")
		}
		if len(req.Env) > 256*1024 {
			return nil, fmt.Errorf(".env is too large")
		}
		path := filepath.Join(root, ".env")
		if err := os.WriteFile(path, []byte(req.Env), 0640); err != nil {
			return nil, err
		}
		own := req.Username + ":" + req.Username
		_ = exec.Command("chown", own, path).Run()
		_ = os.Chmod(path, 0640)
	default:
		return nil, fmt.Errorf("unknown laravel action %q", req.Action)
	}
	out.Laravel = laravelStatus(req.Username, req.Domain, root, req.PHPVersion)
	if req.Action == "composer" || req.Action == "npm" {
		fillPackages(out.Laravel, req.Username, root, req.PHPVersion)
	}
	if req.Action == "artisan" {
		out.Laravel.ArtisanCmds = listArtisan(req.Username, root, req.PHPVersion)
	}
	return out, nil
}

func setLaravelQueue(user, domain, root, phpVer string, on bool, specs []rpc.LaravelQueueRow) error {
	if !on {
		removeLaravelQueueConfs(user, domain)
		return supervisorReload()
	}
	if err := ensureSupervisor(); err != nil {
		return err
	}
	if err := ensureLinuxUser(user); err != nil {
		return err
	}
	if !fileOK(filepath.Join(root, "artisan")) {
		return fmt.Errorf("artisan not found in %s", root)
	}
	if len(specs) == 0 {
		specs = []rpc.LaravelQueueRow{{Name: "default", Workers: 1}}
	}
	_ = os.MkdirAll(filepath.Join("/home", user, "tmp"), 0750)
	_ = exec.Command("chown", user+":"+user, filepath.Join("/home", user, "tmp")).Run()
	if err := os.MkdirAll("/etc/supervisor/conf.d", 0755); err != nil {
		return err
	}
	keep := map[string]bool{}
	php := phpBinForUser(user, phpVer)
	home := filepath.Join("/home", user)
	pathEnv := filepath.Join(home, ".local", "bin") + ":/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	for _, spec := range specs {
		prog := laravelQueueProgram(user, domain, spec.Name)
		keep[prog] = true
		log := filepath.Join(home, "tmp", "queue-"+domain+"-"+spec.Name+".log")
		conf := fmt.Sprintf(`[program:%s]
command=%s artisan queue:work --sleep=3 --tries=3 --timeout=90 --queue=%s
directory=%s
user=%s
numprocs=%d
process_name=%%(program_name)s_%%(process_num)02d
autostart=true
autorestart=true
redirect_stderr=true
stdout_logfile=%s
stopwaitsecs=30
environment=HOME="%s",USER="%s",LOGNAME="%s",PATH="%s"
`, prog, php, spec.Name, root, user, spec.Workers, log, home, user, user, pathEnv)
		if err := os.WriteFile(laravelQueueSupPath(user, domain, spec.Name), []byte(conf), 0644); err != nil {
			return err
		}
	}
	for _, p := range laravelQueueConfPaths(user, domain) {
		base := strings.TrimSuffix(filepath.Base(p), ".conf")
		if !keep[base] {
			_ = os.Remove(p)
		}
	}
	return supervisorReload()
}

func setLaravelScheduler(user, domain, root, phpVer string, on bool) error {
	path := laravelCronPath(user, domain)
	if !on {
		_ = os.Remove(path)
		return nil
	}
	if !fileOK(filepath.Join(root, "artisan")) {
		return fmt.Errorf("artisan not found in %s", root)
	}
	log := laravelScheduleLog(user, domain)
	_ = os.MkdirAll(filepath.Dir(log), 0750)
	_ = exec.Command("chown", user+":"+user, filepath.Dir(log)).Run()
	line := fmt.Sprintf("* * * * * %s cd %s && %s artisan schedule:run >> %s 2>&1\n", user, root, phpBinForUser(user, phpVer), log)
	if err := os.WriteFile(path, []byte(line), 0644); err != nil {
		return err
	}
	return nil
}

func ensureSupervisor() error {
	if exec.Command("systemctl", "is-active", "--quiet", "supervisor").Run() == nil {
		return nil
	}
	if _, err := exec.LookPath("supervisord"); err == nil {
		_ = exec.Command("systemctl", "enable", "--now", "supervisor").Run()
		_ = exec.Command("systemctl", "enable", "--now", "supervisord").Run()
		return supervisorReload()
	}
	if err := software.AptInstall("supervisor"); err != nil {
		return fmt.Errorf("install supervisor: %w", err)
	}
	_ = exec.Command("systemctl", "enable", "--now", "supervisor").Run()
	return supervisorReload()
}

func supervisorReload() error {
	if _, err := exec.LookPath("supervisorctl"); err != nil {
		return nil
	}
	_ = exec.Command("systemctl", "enable", "--now", "supervisor").Run()
	if out, err := exec.Command("supervisorctl", "reread").CombinedOutput(); err != nil {
		return fmt.Errorf("supervisor reread: %s", strings.TrimSpace(string(out)))
	}
	_ = exec.Command("supervisorctl", "update").Run()
	return nil
}

func ReloadSupervisor() error {
	return supervisorReload()
}

func ensureLinuxUser(name string) error {
	if err := validate.LinuxUser(name); err != nil {
		return err
	}
	if _, err := user.Lookup(name); err == nil {
		return nil
	}
	home := "/home"
	if _, err := (&users.Manager{HomeRoot: home}).Ensure(name, 0, 0, ""); err != nil {
		return fmt.Errorf("linux user %s is missing: %w", name, err)
	}
	return nil
}

func cleanupLaravel(user, domain string) {
	removeLaravelQueueConfs(user, domain)
	_ = os.Remove(laravelCronPath(user, domain))
	_ = supervisorReload()
}

func renameLaravel(user, oldDomain, newDomain string) {
	oldNeedle := "/domains/" + oldDomain + "/"
	newNeedle := "/domains/" + newDomain + "/"
	for _, p := range laravelQueueConfPaths(user, oldDomain) {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		s := string(b)
		s = strings.ReplaceAll(s, laravelProgram(user, oldDomain), laravelProgram(user, newDomain))
		s = strings.ReplaceAll(s, "siroc-"+user+"-"+slug(oldDomain)+"-q-", "siroc-"+user+"-"+slug(newDomain)+"-q-")
		s = strings.ReplaceAll(s, "queue-"+oldDomain, "queue-"+newDomain)
		s = strings.ReplaceAll(s, "schedule-"+oldDomain, "schedule-"+newDomain)
		s = strings.ReplaceAll(s, oldNeedle, newNeedle)
		name := filepath.Base(p)
		name = strings.ReplaceAll(name, slug(oldDomain), slug(newDomain))
		_ = os.WriteFile(filepath.Join("/etc/supervisor/conf.d", name), []byte(s), 0644)
		if filepath.Base(p) != name {
			_ = os.Remove(p)
		}
	}
	if b, err := os.ReadFile(laravelCronPath(user, oldDomain)); err == nil {
		s := string(b)
		s = strings.ReplaceAll(s, oldNeedle, newNeedle)
		s = strings.ReplaceAll(s, "schedule-"+oldDomain, "schedule-"+newDomain)
		_ = os.WriteFile(laravelCronPath(user, newDomain), []byte(s), 0644)
		_ = os.Remove(laravelCronPath(user, oldDomain))
	}
	_ = supervisorReload()
}

func laravelQueueConfPaths(user, domain string) []string {
	var out []string
	if p := laravelSupPath(user, domain); fileOK(p) {
		out = append(out, p)
	}
	matches, _ := filepath.Glob("/etc/supervisor/conf.d/" + "siroc-" + user + "-" + slug(domain) + "-q-*.conf")
	out = append(out, matches...)
	return out
}

func removeLaravelQueueConfs(user, domain string) {
	for _, p := range laravelQueueConfPaths(user, domain) {
		_ = os.Remove(p)
	}
}

func readLaravelQueueRows(user, domain string) []rpc.LaravelQueueRow {
	var rows []rpc.LaravelQueueRow
	for _, p := range laravelQueueConfPaths(user, domain) {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		conf := string(b)
		names := strings.Split(laravelQueueNameFromConf(conf), ",")
		workers := laravelWorkersFromConf(conf)
		log := laravelLogFromConf(conf)
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			rows = append(rows, rpc.LaravelQueueRow{Name: name, Workers: workers, Log: log})
		}
	}
	return rows
}

func attachQueueRuntime(user, domain string, rows []rpc.LaravelQueueRow) []rpc.LaravelQueueRow {
	now := time.Now()
	procs := supervisorStatusMap()
	out := make([]rpc.LaravelQueueRow, 0, len(rows))
	for _, row := range rows {
		prog := laravelQueueProgram(user, domain, row.Name)
		legacy := laravelProgram(user, domain)
		var matched []rpc.LaravelQueueProc
		running := 0
		status := "STOPPED"
		var last time.Time
		if t, err := os.Stat(row.Log); err == nil {
			last = t.ModTime()
		}
		for _, p := range procs {
			if p.Name != prog && !strings.HasPrefix(p.Name, prog+"_") && p.Name != legacy && !strings.HasPrefix(p.Name, legacy+"_") {
				continue
			}
			started := lastRunFromUptime(now, p.Uptime)
			if !started.IsZero() && (last.IsZero() || started.After(last)) {
				last = started
			}
			if strings.EqualFold(p.Status, "RUNNING") {
				running++
			}
			if status == "STOPPED" || strings.EqualFold(p.Status, "RUNNING") || strings.EqualFold(p.Status, "BACKOFF") || strings.EqualFold(p.Status, "FATAL") {
				status = p.Status
			}
			item := rpc.LaravelQueueProc{
				Name:    p.Name,
				Queue:   row.Name,
				Status:  p.Status,
				PID:     p.PID,
				Uptime:  p.Uptime,
				LastRun: formatRunTime(started),
				NextRun: queueNextRun(p.Status, started, now),
			}
			if item.LastRun == "" {
				item.LastRun = formatRunTime(last)
			}
			matched = append(matched, item)
		}
		if status == "STOPPED" && running == 0 && row.Workers > 0 && fileOK(laravelQueueSupPath(user, domain, row.Name)) {
			status = "configured"
		}
		row.Running = running
		row.Status = status
		row.LastRun = formatRunTime(last)
		row.NextRun = queueNextRun(status, last, now)
		if row.NextRun == "" && strings.EqualFold(status, "configured") {
			row.NextRun = "waiting"
		}
		row.Processes = matched
		out = append(out, row)
	}
	return out
}

func supervisorStatusMap() []supervisorProc {
	if _, err := exec.LookPath("supervisorctl"); err != nil {
		return nil
	}
	out, err := exec.Command("supervisorctl", "status").CombinedOutput()
	if err != nil && len(out) == 0 {
		return nil
	}
	return parseSupervisorStatus(string(out))
}

func phpBinForUser(user, ver string) string {
	if p := phpBin(ver); p != "php" {
		return p
	}
	local := filepath.Join("/home", user, ".local", "bin", "php")
	if fileOK(local) {
		return local
	}
	return "php"
}

func fileModRFC3339(path string) string {
	st, err := os.Stat(path)
	if err != nil {
		return ""
	}
	return formatRunTime(st.ModTime())
}

func laravelScheduleList(user, root, phpVer, last string) []rpc.LaravelSchedule {
	if !fileOK(filepath.Join(root, "artisan")) {
		return nil
	}
	php := phpBinForUser(user, phpVer)
	raw, err := runAs(user, root, 30*time.Second, shellQuote(php)+" artisan schedule:list --json --no-interaction --no-ansi")
	if err != nil || strings.TrimSpace(raw) == "" || !strings.Contains(raw, "expression") {
		raw, err = runAs(user, root, 30*time.Second, shellQuote(php)+" artisan schedule:list --no-interaction --no-ansi")
		if err != nil {
			return nil
		}
	}
	events := parseScheduleList(raw)
	for i := range events {
		events[i].LastRun = last
	}
	return events
}

func ensureComposer() error {
	if _, err := exec.LookPath("composer"); err == nil {
		return nil
	}
	home := "/var/lib/siroc/composer"
	_ = os.MkdirAll(home, 0750)
	setup := "/var/lib/siroc/composer-setup.php"
	if err := exec.Command("curl", "-fsSL", "-o", setup, "https://getcomposer.org/installer").Run(); err != nil {
		return fmt.Errorf("download composer installer: %w", err)
	}
	defer os.Remove(setup)
	cmd := exec.Command("php", setup, "--install-dir=/usr/local/bin", "--filename=composer")
	cmd.Env = append(os.Environ(), "HOME=/root", "COMPOSER_HOME="+home, "DEBIAN_FRONTEND=noninteractive")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("install composer: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return os.Chmod("/usr/local/bin/composer", 0755)
}

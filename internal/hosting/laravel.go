//go:build linux

package hosting

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/software"
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

func laravelStatus(user, domain, root, phpVer string) *rpc.LaravelStatus {
	st := &rpc.LaravelStatus{
		HasArtisan:  fileOK(filepath.Join(root, "artisan")),
		HasComposer: fileOK(filepath.Join(root, "composer.json")),
		HasPackage:  fileOK(filepath.Join(root, "package.json")),
		EnvExists:   fileOK(filepath.Join(root, ".env")),
		Queue:       fileOK(laravelSupPath(user, domain)),
		Scheduler:   fileOK(laravelCronPath(user, domain)),
		Workers:     1,
	}
	if b, err := os.ReadFile(laravelSupPath(user, domain)); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "numprocs=") {
				n, _ := strconv.Atoi(strings.TrimPrefix(line, "numprocs="))
				if n > 0 {
					st.Workers = n
				}
			}
		}
	}
	_ = phpVer
	return st
}

func laravelAction(req rpc.SiteAppReq, out *rpc.SiteAppResp) (*rpc.SiteAppResp, error) {
	root := out.AppRoot
	switch req.Action {
	case "queue":
		on := req.Queue != nil && *req.Queue
		workers := req.Workers
		if workers < 1 {
			workers = 1
		}
		if workers > 8 {
			workers = 8
		}
		if err := setLaravelQueue(req.Username, req.Domain, root, req.PHPVersion, on, workers); err != nil {
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

func setLaravelQueue(user, domain, root, phpVer string, on bool, workers int) error {
	path := laravelSupPath(user, domain)
	if !on {
		_ = os.Remove(path)
		return supervisorReload()
	}
	if err := ensureSupervisor(); err != nil {
		return err
	}
	if !fileOK(filepath.Join(root, "artisan")) {
		return fmt.Errorf("artisan not found in %s", root)
	}
	log := filepath.Join("/home", user, "tmp", "queue-"+domain+".log")
	_ = os.MkdirAll(filepath.Dir(log), 0750)
	_ = exec.Command("chown", user+":"+user, filepath.Dir(log)).Run()
	conf := fmt.Sprintf(`[program:%s]
command=%s artisan queue:work --sleep=3 --tries=3 --timeout=90
directory=%s
user=%s
numprocs=%d
process_name=%%(program_name)s_%%(process_num)02d
autostart=true
autorestart=true
redirect_stderr=true
stdout_logfile=%s
stopwaitsecs=30
`, laravelProgram(user, domain), phpBin(phpVer), root, user, workers, log)
	if err := os.MkdirAll("/etc/supervisor/conf.d", 0755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(conf), 0644); err != nil {
		return err
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
	line := fmt.Sprintf("* * * * * %s cd %s && %s artisan schedule:run >/dev/null 2>&1\n", user, root, phpBin(phpVer))
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

func cleanupLaravel(user, domain string) {
	_ = os.Remove(laravelSupPath(user, domain))
	_ = os.Remove(laravelCronPath(user, domain))
	_ = supervisorReload()
}

func renameLaravel(user, oldDomain, newDomain string) {
	oldNeedle := "/domains/" + oldDomain + "/"
	newNeedle := "/domains/" + newDomain + "/"
	if b, err := os.ReadFile(laravelSupPath(user, oldDomain)); err == nil {
		s := string(b)
		s = strings.ReplaceAll(s, laravelProgram(user, oldDomain), laravelProgram(user, newDomain))
		s = strings.ReplaceAll(s, "queue-"+oldDomain+".log", "queue-"+newDomain+".log")
		s = strings.ReplaceAll(s, oldNeedle, newNeedle)
		_ = os.WriteFile(laravelSupPath(user, newDomain), []byte(s), 0644)
		_ = os.Remove(laravelSupPath(user, oldDomain))
	}
	if b, err := os.ReadFile(laravelCronPath(user, oldDomain)); err == nil {
		s := string(b)
		s = strings.ReplaceAll(s, oldNeedle, newNeedle)
		_ = os.WriteFile(laravelCronPath(user, newDomain), []byte(s), 0644)
		_ = os.Remove(laravelCronPath(user, oldDomain))
	}
	_ = supervisorReload()
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

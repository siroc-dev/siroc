//go:build linux

package hosting

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/validate"
)

func phpBin(ver string) string {
	p := "/usr/bin/php" + ver
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return "php"
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func runAs(username, dir string, timeout time.Duration, script string) (string, error) {
	if err := validate.LinuxUser(username); err != nil {
		return "", err
	}
	home := filepath.Join("/home", username)
	path := filepath.Join(home, ".local", "bin") + ":/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	inner := "export PATH=" + shellQuote(path) + "; export HOME=" + shellQuote(home) + "; cd " + shellQuote(dir) + " && " + script
	cmd := exec.Command("runuser", "-u", username, "--", "bash", "-lc", inner)
	cmd.Env = overlayEnv(map[string]string{
		"HOME":             home,
		"USER":             username,
		"LOGNAME":          username,
		"PATH":             path,
		"npm_config_cache": filepath.Join(home, ".npm"),
	})
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return "", err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		out := strings.TrimSpace(stdout.String())
		errOut := strings.TrimSpace(stderr.String())
		if err != nil {
			msg := out
			if errOut != "" {
				if msg != "" {
					msg += "\n"
				}
				msg += errOut
			}
			if msg == "" {
				msg = err.Error()
			}
			return msg, fmt.Errorf("%s", tailOut(msg))
		}
		return out, nil
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		return stdout.String(), fmt.Errorf("timed out after %s", timeout)
	}
}

func overlayEnv(kv map[string]string) []string {
	env := os.Environ()
	idx := map[string]int{}
	for i, e := range env {
		if k, _, ok := strings.Cut(e, "="); ok {
			idx[k] = i
		}
	}
	for k, v := range kv {
		item := k + "=" + v
		if i, ok := idx[k]; ok {
			env[i] = item
		} else {
			env = append(env, item)
		}
	}
	return env
}

func resolveUserBin(username, name string) string {
	local := filepath.Join("/home", username, ".local", "bin", name)
	if fileOK(local) {
		return local
	}
	if name == "npm" || name == "npx" {
		if dir := nodeBinDir(username); dir != "" {
			p := filepath.Join(dir, name)
			if fileOK(p) {
				return p
			}
		}
		if entries, err := os.ReadDir("/opt/siroc-runtimes/node"); err == nil {
			for i := len(entries) - 1; i >= 0; i-- {
				p := filepath.Join("/opt/siroc-runtimes/node", entries[i].Name(), "bin", name)
				if fileOK(p) {
					return p
				}
			}
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return name
}

func nodeBinDir(username string) string {
	b, err := os.ReadFile(filepath.Join("/home", username, ".local", "bin", "node"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "exec ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			return filepath.Dir(fields[1])
		}
	}
	return ""
}

func prefixBin(name, bin, script string) string {
	if bin == "" || bin == name {
		return script
	}
	if script == name || strings.HasPrefix(script, name+" ") {
		return shellQuote(bin) + strings.TrimPrefix(script, name)
	}
	return script
}

func detectKind(doc string) (kind, appRoot string) {
	doc = filepath.Clean(doc)
	if fileOK(filepath.Join(doc, "wp-config.php")) || fileOK(filepath.Join(doc, "wp-load.php")) {
		return "wordpress", doc
	}
	parent := filepath.Dir(doc)
	if fileOK(filepath.Join(parent, "wp-config.php")) {
		return "wordpress", parent
	}
	if fileOK(filepath.Join(doc, "artisan")) {
		return "laravel", doc
	}
	if fileOK(filepath.Join(parent, "artisan")) {
		return "laravel", parent
	}
	return "php", doc
}

func (m *Manager) SiteApp(req rpc.SiteAppReq) (*rpc.SiteAppResp, error) {
	if err := validate.LinuxUser(req.Username); err != nil {
		return nil, err
	}
	if err := validate.Domain(req.Domain); err != nil {
		return nil, err
	}
	doc, err := validate.AccountPath(m.HomeRoot, req.Username, req.DocRoot, req.Domain)
	if err != nil {
		return nil, err
	}
	kind, root := detectKind(doc)
	out := &rpc.SiteAppResp{OK: true, Kind: kind, AppRoot: root, DocRoot: doc}
	action := strings.TrimSpace(req.Action)
	if action == "install-wordpress" {
		return wpInstall(req, out)
	}
	if action == "" || action == "status" {
		switch kind {
		case "laravel":
			out.Laravel = laravelStatus(req.Username, req.Domain, root, req.PHPVersion)
		case "wordpress":
			wp, err := wpStatus(req.Username, root, req.Domain)
			if err != nil {
				out.Message = err.Error()
			}
			if wp != nil {
				wp.WPCmds = listWPCLI(req.Username, root)
			}
			out.WordPress = wp
		}
		return out, nil
	}
	switch kind {
	case "laravel":
		return laravelAction(req, out)
	case "wordpress":
		return wpAction(req, out)
	default:
		out.Message = "this document root is not a Laravel or WordPress app"
		return out, nil
	}
}

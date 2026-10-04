//go:build linux

package weblog

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/validate"
)

// ClearSiteLogs empties one log, or every log for the site when ID is empty.
// Live web-server files are truncated so Nginx and Apache keep the same inode, then reopened.
func ClearSiteLogs(req rpc.SiteLogReq) (*rpc.SiteLogsResp, error) {
	dom, err := validate.ASCIIHost(req.Domain)
	if err != nil {
		return nil, err
	}
	if err := validate.LinuxUser(req.Username); err != nil {
		return nil, err
	}
	req.Domain = dom
	req.Probe = false
	id := strings.TrimSpace(req.ID)
	web := false
	if id == "" || id == "all" {
		if err := clearAllSiteLogs(req); err != nil {
			return nil, err
		}
		web = true
		req.ID = ""
	} else {
		group, name, ok := ParseSiteLogID(id)
		if !ok {
			return nil, fmt.Errorf("unknown log %q", id)
		}
		if err := clearOneSiteLog(req, group, name); err != nil {
			return nil, err
		}
		web = group != "laravel"
	}
	if web {
		reopenSiteLogs()
	}
	return SiteLogs(req)
}

func clearAllSiteLogs(req rpc.SiteLogReq) error {
	for _, group := range []string{"nginx", "apache", "waf"} {
		names := []string{"access", "error"}
		if group == "waf" {
			names = []string{"audit"}
		}
		for _, name := range names {
			if err := clearOneSiteLog(req, group, name); err != nil {
				return err
			}
		}
	}
	return clearLaravelDir(req, "")
}

func clearOneSiteLog(req rpc.SiteLogReq, group, name string) error {
	if group == "laravel" {
		return clearLaravelDir(req, name)
	}
	for _, path := range webLogPaths(req.Domain, group, name) {
		if err := truncateLog(path); err != nil {
			return err
		}
		if err := removeRotated(filepath.Dir(path), req.Domain, filepath.Base(path)); err != nil {
			return err
		}
	}
	if group != "waf" && name == "access" {
		_ = os.Remove(filepath.Join(goaccessDir, req.Domain+".html"))
	}
	return nil
}

func webLogPaths(domain, group, name string) []string {
	switch group {
	case "nginx", "apache":
		kind := "access"
		if name == "error" {
			kind = "error"
		}
		primary, leftover := leftoverFor(group, kind, domain)
		if leftover == "" || leftover == primary {
			return []string{primary}
		}
		return []string{primary, leftover}
	case "waf":
		return []string{ApacheWAFLog(domain)}
	}
	return nil
}

func clearLaravelDir(req rpc.SiteLogReq, only string) error {
	dir := laravelLogDir(req.Username, req.DocRoot)
	if dir == "" {
		return nil
	}
	if only != "" {
		if !ValidLaravelLogName(only) {
			return fmt.Errorf("unknown log")
		}
		path := filepath.Join(dir, only)
		if !InHome(filepath.Join("/home", req.Username), path) {
			return fmt.Errorf("log is outside the account home")
		}
		return truncateLog(path)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range ents {
		if e.IsDir() || !ValidLaravelLogName(e.Name()) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if !InHome(filepath.Join("/home", req.Username), path) {
			continue
		}
		if err := truncateLog(path); err != nil {
			return err
		}
	}
	return nil
}

func removeRotated(dir, domain, liveBase string) error {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	prefix := liveBase + "-"
	for _, e := range ents {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) || SiteLogFileKind(domain, name) != "rotated" {
			continue
		}
		path := filepath.Join(dir, name)
		st, err := os.Lstat(path)
		if err != nil || !st.Mode().IsRegular() {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func truncateLog(path string) error {
	st, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !st.Mode().IsRegular() {
		return nil
	}
	return os.Truncate(path, 0)
}

func reopenSiteLogs() {
	reopenNginxLogs()
	if _, err := os.Stat("/run/apache2/apache2.pid"); err == nil {
		_ = exec.Command("systemctl", "reload", "apache2").Run()
	}
}

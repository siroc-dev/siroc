package weblog

import (
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func leftoverFor(group, kind, domain string) (primary, leftover string) {
	switch group {
	case "nginx":
		if kind == "error" {
			return "/var/log/nginx/sites/" + domain + "-error.log", "/var/log/nginx/" + domain + "-error.log"
		}
		return "/var/log/nginx/sites/" + domain + "-access.log", "/var/log/nginx/" + domain + "-access.log"
	case "apache":
		if kind == "error" {
			return "/var/log/apache2/sites/" + domain + "-error.log", "/var/log/apache2/" + domain + "-error.log"
		}
		return "/var/log/apache2/sites/" + domain + "-access.log", "/var/log/apache2/" + domain + "-access.log"
	}
	return "", ""
}

func domainFromLogPath(path string) string {
	base := path
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		base = path[i+1:]
	}
	for _, suf := range []string{"-access.log", "-error.log", "-modsec.log"} {
		if strings.HasSuffix(base, suf) {
			return strings.TrimSuffix(base, suf)
		}
	}
	return ""
}

func logHint(req rpc.SiteLogReq, files []rpc.SiteLogFile, current *rpc.SiteLogFile, hasContent bool, probeErr error) string {
	if current != nil && current.Path != "" {
		if _, alt := leftoverFor(current.Group, current.Kind, req.Domain); alt != "" && current.Path == alt {
			return "Reading an older log path. Nginx is now writing to /var/log/nginx/sites/."
		}
	}
	if hasContent {
		if req.Probe {
			return "Recorded a local test request to this domain."
		}
		return ""
	}
	if req.Probe && probeErr != nil {
		return "Local test request could not reach Nginx on this server. Confirm nginx is running and this domain’s vhost is enabled."
	}
	empty := true
	for _, f := range files {
		if f.Exists && f.Size > 0 {
			empty = false
			break
		}
	}
	if !empty {
		return ""
	}
	return "These files are created empty when the site is added. They stay at 0 B until a request hits this hostname on this server. Opening the panel does not write access logs. Apache fills only after Nginx proxies a visit. If Cloudflare or DNS points elsewhere, nothing is recorded here."
}

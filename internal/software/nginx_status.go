//go:build linux

package software

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

const nginxStubConf = `/etc/nginx/sites-available/cp-stub-status.conf`
const nginxStubListen = "127.0.0.1:8180"

func NginxStatus() *rpc.NginxStatus {
	st := &rpc.NginxStatus{}
	ok, ver := dpkgVersion("nginx")
	st.Installed = ok
	st.Version = nginxBinaryVersion()
	if st.Version == "" {
		st.Version = ver
	}
	if !ok && st.Version == "" {
		st.Message = "Nginx is not installed. Install it from Software."
		return st
	}
	st.Installed = true
	st.Active = serviceActive("nginx")
	_ = enableNginxStatus()
	if !st.Active {
		st.Message = "Nginx is installed but not running."
		st.Sites = nginxVHosts()
		return st
	}
	st.UptimeSec = serviceUptimeSec("nginx")
	st.WorkerProcesses, st.WorkerConnections = nginxWorkers()
	raw, err := fetchURL("http://" + nginxStubListen + "/nginx-status")
	if err != nil {
		st.Message = "stub_status is not reachable yet. " + err.Error()
		st.Sites = nginxVHosts()
		return st
	}
	parseNginxStub(st, raw)
	if st.UptimeSec > 0 {
		st.ReqPerSec = float64(st.Requests) / float64(st.UptimeSec)
	}
	st.Sites = nginxVHosts()
	st.Ready = true
	st.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	return st
}

func enableNginxStatus() error {
	if _, err := exec.LookPath("nginx"); err != nil {
		return err
	}
	_ = os.MkdirAll("/etc/nginx/sites-available", 0755)
	_ = os.MkdirAll("/etc/nginx/sites-enabled", 0755)
	_ = os.MkdirAll("/etc/nginx/snippets", 0755)
	body := `server {
    listen ` + nginxStubListen + `;
    server_name localhost;
    access_log off;
    allow 127.0.0.1;
    deny all;
    location = /nginx-status {
        stub_status;
    }
    location / { return 404; }
}
`
	changed := writeIfChanged(nginxStubConf, body)
	link := "/etc/nginx/sites-enabled/cp-stub-status.conf"
	if _, err := os.Lstat(link); err != nil {
		_ = os.Remove(link)
		if err := os.Symlink(nginxStubConf, link); err == nil {
			changed = true
		}
	}
	_ = writeIfChanged(nginxStatusDeny, "location ^~ /server-status { return 404; }\nlocation ^~ /nginx-status { return 404; }\nlocation ^~ /fpm-status { return 404; }\n")
	denyPublicServerStatus()
	if changed {
		if err := exec.Command("nginx", "-t").Run(); err != nil {
			return err
		}
		_ = exec.Command("systemctl", "reload", "nginx").Run()
	}
	return nil
}

func nginxBinaryVersion() string {
	cmd := exec.Command("nginx", "-v")
	out, _ := cmd.CombinedOutput()
	s := strings.TrimSpace(string(out))
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return "nginx/" + strings.TrimSpace(s[i+1:])
	}
	return s
}

func fetchURL(url string) (string, error) {
	cli := &http.Client{Timeout: 2 * time.Second}
	res, err := cli.Get(url)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return string(b), nil
}

func parseNginxStub(st *rpc.NginxStatus, raw string) {
	if m := regexp.MustCompile(`Active connections:\s+(\d+)`).FindStringSubmatch(raw); len(m) == 2 {
		st.ActiveConn = atoi(m[1])
	}
	if m := regexp.MustCompile(`(?m)^\s*(\d+)\s+(\d+)\s+(\d+)\s*$`).FindStringSubmatch(raw); len(m) == 4 {
		st.Accepts = atoi64(m[1])
		st.Handled = atoi64(m[2])
		st.Requests = atoi64(m[3])
	}
	if m := regexp.MustCompile(`Reading:\s+(\d+)\s+Writing:\s+(\d+)\s+Waiting:\s+(\d+)`).FindStringSubmatch(raw); len(m) == 4 {
		st.Reading = atoi(m[1])
		st.Writing = atoi(m[2])
		st.Waiting = atoi(m[3])
	}
}

func nginxWorkers() (string, int) {
	b, err := os.ReadFile("/etc/nginx/nginx.conf")
	if err != nil {
		return "", 0
	}
	s := string(b)
	procs := ""
	if m := regexp.MustCompile(`(?m)^\s*worker_processes\s+(\S+);`).FindStringSubmatch(s); len(m) == 2 {
		procs = m[1]
	}
	conns := 0
	if m := regexp.MustCompile(`(?m)^\s*worker_connections\s+(\d+);`).FindStringSubmatch(s); len(m) == 2 {
		conns = atoi(m[1])
	}
	return procs, conns
}

func nginxVHosts() []rpc.NginxVHost {
	files, _ := filepath.Glob("/etc/nginx/sites-enabled/*")
	var out []rpc.NginxVHost
	seen := map[string]bool{}
	nameRe := regexp.MustCompile(`(?m)^\s*server_name\s+([^;]+);`)
	listenRe := regexp.MustCompile(`(?m)^\s*listen\s+([^;]+);`)
	for _, f := range files {
		base := filepath.Base(f)
		if strings.Contains(base, "stub-status") || strings.Contains(base, "siroc-default") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		s := string(b)
		ssl := strings.Contains(s, "listen 443") || strings.Contains(s, "ssl_certificate")
		listen := ""
		if m := listenRe.FindStringSubmatch(s); len(m) == 2 {
			listen = strings.TrimSpace(m[1])
		}
		names := []string{}
		if m := nameRe.FindStringSubmatch(s); len(m) == 2 {
			for _, n := range strings.Fields(m[1]) {
				n = strings.TrimSpace(n)
				if n == "" || n == "_" || n == "localhost" {
					continue
				}
				names = append(names, n)
			}
		}
		if len(names) == 0 {
			names = []string{base}
		}
		for _, n := range names {
			key := n + "|" + listen
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, rpc.NginxVHost{Name: n, Listen: listen, SSL: ssl, File: f})
		}
	}
	return out
}

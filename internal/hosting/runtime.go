//go:build linux

package hosting

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/validate"
)

func appUnit(user, domain string) string {
	return "siroc-app-" + user + "-" + slug(domain) + ".service"
}

func appUnitPath(user, domain string) string {
	return filepath.Join("/etc/systemd/system", appUnit(user, domain))
}

func appContainer(user, domain string) string {
	return "siroc-" + user + "-" + slug(domain)
}

func (m *Manager) writeApp(req rpc.SiteWriteReq) error {
	kind, err := validate.SiteKind(req.Kind)
	if err != nil || !validate.IsAppKind(kind) {
		return fmt.Errorf("not an app site type")
	}
	cmd, err := validate.AppCommand(req.AppCmd)
	if err != nil {
		cmd, err = validate.AppCommand(validate.DefaultAppCmd(kind))
		if err != nil {
			return err
		}
	}
	if err := validate.AppPort(req.AppPort); err != nil {
		return err
	}
	doc := req.DocRoot
	if strings.TrimSpace(doc) == "" {
		doc = filepath.Join(m.HomeRoot, req.Username, "domains", req.Domain, "public_html")
	} else {
		doc, err = validate.AccountPath(m.HomeRoot, req.Username, req.DocRoot, req.Domain)
		if err != nil {
			return err
		}
	}
	if err := os.MkdirAll(doc, 0755); err != nil {
		return err
	}
	if err := seedApp(kind, doc); err != nil {
		return err
	}
	_ = exec.Command("chown", "-R", req.Username+":"+req.Username, doc).Run()
	if kind == "docker" {
		_ = exec.Command("usermod", "-aG", "docker", req.Username).Run()
	}
	home := filepath.Join(m.HomeRoot, req.Username)
	quoted := strconv.Quote(cmd)
	body := fmt.Sprintf(`[Unit]
Description=Siroc app %s (%s)
After=network.target docker.service
Wants=network.target

[Service]
Type=simple
User=%s
Group=%s
%sWorkingDirectory=%s
Environment=HOME=%s
Environment=PORT=%d
Environment=HOST=127.0.0.1
Environment=PATH=%s/.cargo/bin:%s/.local/bin:/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin
ExecStart=/bin/bash -lc %s
Restart=on-failure
RestartSec=3
TimeoutStartSec=180
StandardOutput=journal
StandardError=journal
SyslogIdentifier=%s

[Install]
WantedBy=multi-user.target
`, req.Domain, kind, req.Username, req.Username, dockerGroups(kind), doc, home, req.AppPort, home, home, quoted, strings.TrimSuffix(appUnit(req.Username, req.Domain), ".service"))
	if err := os.WriteFile(appUnitPath(req.Username, req.Domain), []byte(body), 0644); err != nil {
		return err
	}
	_ = exec.Command("systemctl", "daemon-reload").Run()
	if req.Enabled {
		if out, err := exec.Command("systemctl", "enable", "--now", appUnit(req.Username, req.Domain)).CombinedOutput(); err != nil {
			return fmt.Errorf("start app: %s", strings.TrimSpace(string(out)))
		}
	} else {
		_ = exec.Command("systemctl", "disable", "--now", appUnit(req.Username, req.Domain)).Run()
	}
	req.Kind = "proxy"
	req.ProxyPass = fmt.Sprintf("http://127.0.0.1:%d/", req.AppPort)
	req.AppCmd = cmd
	return m.writeProxy(req)
}

func dockerGroups(kind string) string {
	if kind == "docker" {
		return "SupplementaryGroups=docker\n"
	}
	return ""
}

func dropApp(user, domain string) {
	unit := appUnit(user, domain)
	_ = exec.Command("systemctl", "disable", "--now", unit).Run()
	_ = os.Remove(appUnitPath(user, domain))
	_ = exec.Command("docker", "rm", "-f", appContainer(user, domain)).Run()
	_ = exec.Command("systemctl", "daemon-reload").Run()
}

func renameApp(user, old, newDom string) {
	dropApp(user, old)
}

func (m *Manager) SiteRuntime(req rpc.SiteRuntimeReq) (*rpc.SiteRuntimeResp, error) {
	if err := validate.LinuxUser(req.Username); err != nil {
		return nil, err
	}
	if err := validate.Domain(req.Domain); err != nil {
		return nil, err
	}
	kind, err := validate.SiteKind(req.Kind)
	if err != nil || !validate.IsAppKind(kind) {
		return nil, fmt.Errorf("not an app site type")
	}
	unit := appUnit(req.Username, req.Domain)
	out := &rpc.SiteRuntimeResp{OK: true, Kind: kind, Port: req.AppPort, Cmd: req.AppCmd, Unit: unit}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	switch action {
	case "", "status", "logs":
	case "start":
		if b, err := exec.Command("systemctl", "enable", "--now", unit).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("start: %s", strings.TrimSpace(string(b)))
		}
	case "stop":
		_ = exec.Command("systemctl", "stop", unit).Run()
	case "restart":
		if b, err := exec.Command("systemctl", "restart", unit).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("restart: %s", strings.TrimSpace(string(b)))
		}
	default:
		return nil, fmt.Errorf("unknown runtime action")
	}
	st := strings.TrimSpace(string(mustCombined("systemctl", "is-active", unit)))
	out.Status = st
	out.Active = st == "active"
	n := "80"
	if action == "logs" {
		n = "200"
	}
	logb, _ := exec.Command("journalctl", "-u", unit, "-n", n, "--no-pager", "-o", "cat").CombinedOutput()
	out.Log = strings.TrimSpace(string(logb))
	if len(out.Log) > 20000 {
		out.Log = out.Log[len(out.Log)-20000:]
	}
	return out, nil
}

func mustCombined(name string, args ...string) []byte {
	b, _ := exec.Command(name, args...).CombinedOutput()
	return b
}

func seedApp(kind, doc string) error {
	switch kind {
	case "nodejs":
		if fileOK(filepath.Join(doc, "server.js")) || fileOK(filepath.Join(doc, "package.json")) {
			return nil
		}
		if err := os.WriteFile(filepath.Join(doc, "package.json"), []byte("{\n  \"name\": \"site\",\n  \"private\": true,\n  \"scripts\": { \"start\": \"node server.js\" }\n}\n"), 0644); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(doc, "server.js"), []byte(`const http = require("http");
const port = Number(process.env.PORT || 3000);
http.createServer((req, res) => {
  res.writeHead(200, { "Content-Type": "text/plain; charset=utf-8" });
  res.end("Node.js site is running\n");
}).listen(port, "127.0.0.1");
`), 0644)
	case "python":
		if fileOK(filepath.Join(doc, "app.py")) {
			return nil
		}
		return os.WriteFile(filepath.Join(doc, "app.py"), []byte(`from http.server import BaseHTTPRequestHandler, HTTPServer
import os

class H(BaseHTTPRequestHandler):
    def do_GET(self):
        body = b"Python site is running\n"
        self.send_response(200)
        self.send_header("Content-Type", "text/plain; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, fmt, *args):
        return

HTTPServer(("127.0.0.1", int(os.environ.get("PORT", "8000"))), H).serve_forever()
`), 0644)
	case "go":
		if fileOK(filepath.Join(doc, "main.go")) || fileOK(filepath.Join(doc, "go.mod")) {
			return nil
		}
		if err := os.WriteFile(filepath.Join(doc, "go.mod"), []byte("module site\n\ngo 1.22\n"), 0644); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(doc, "main.go"), []byte(`package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	p := os.Getenv("PORT")
	if p == "" {
		p = "8080"
	}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Go site is running\n")
	})
	http.ListenAndServe("127.0.0.1:"+p, nil)
}
`), 0644)
	case "rust":
		if fileOK(filepath.Join(doc, "Cargo.toml")) {
			return nil
		}
		if err := os.MkdirAll(filepath.Join(doc, "src"), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(doc, "Cargo.toml"), []byte("[package]\nname = \"site\"\nversion = \"0.1.0\"\nedition = \"2021\"\n"), 0644); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(doc, "src/main.rs"), []byte(`use std::env;
use std::io::Write;
use std::net::TcpListener;

fn main() {
    let port = env::var("PORT").unwrap_or_else(|_| "8080".into());
    let listener = TcpListener::bind(format!("127.0.0.1:{port}")).expect("bind");
    for stream in listener.incoming() {
        if let Ok(mut s) = stream {
            let body = "Rust site is running\n";
            let _ = write!(s, "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}", body.len(), body);
        }
    }
}
`), 0644)
	case "docker":
		if fileOK(filepath.Join(doc, "Dockerfile")) || fileOK(filepath.Join(doc, "docker-compose.yml")) || fileOK(filepath.Join(doc, "compose.yml")) {
			return nil
		}
		if err := os.WriteFile(filepath.Join(doc, "app.py"), []byte(`from http.server import BaseHTTPRequestHandler, HTTPServer
import os

class H(BaseHTTPRequestHandler):
    def do_GET(self):
        body = b"Docker site is running\n"
        self.send_response(200)
        self.send_header("Content-Type", "text/plain; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, fmt, *args):
        return

HTTPServer(("0.0.0.0", int(os.environ.get("PORT", "8080"))), H).serve_forever()
`), 0644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(doc, "Dockerfile"), []byte("FROM python:3.12-alpine\nWORKDIR /app\nCOPY . .\nCMD [\"python3\", \"app.py\"]\n"), 0644); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(doc, "docker-compose.yml"), []byte(`services:
  web:
    build: .
    ports:
      - "127.0.0.1:${PORT}:${PORT}"
    environment:
      PORT: ${PORT}
`), 0644)
	}
	return nil
}

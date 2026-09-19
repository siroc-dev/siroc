//go:build linux

package software

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func PHPFPMStatus() *rpc.PHPFPMStatus {
	st := &rpc.PHPFPMStatus{}
	vers := phpFPMVersions()
	st.Versions = vers
	st.Installed = len(vers) > 0
	if !st.Installed {
		st.Message = "PHP-FPM is not installed. Install PHP from Software."
		return st
	}
	for _, v := range vers {
		if serviceActive("php" + v + "-fpm") {
			st.Active = true
			break
		}
	}
	_ = enablePHPFPMStatus()
	if !st.Active {
		st.Message = "PHP-FPM is installed but not running."
		return st
	}
	pools := listFPMPools()
	for i := range pools {
		p := &pools[i]
		if p.Listen == "" {
			p.Message = "No listen socket"
			continue
		}
		raw, err := fpmQuery(p.Listen, "/fpm-status")
		if err != nil {
			p.Message = err.Error()
			continue
		}
		if err := parseFPMJSON(p, raw); err != nil {
			p.Message = err.Error()
			continue
		}
		p.Ready = true
		st.Idle += p.Idle
		st.ActiveProcs += p.Active
		st.Total += p.Total
		st.Accepted += p.Accepted
		st.Ready = true
	}
	st.Pools = pools
	if !st.Ready {
		st.Message = "No PHP-FPM pool answered /fpm-status yet."
	}
	st.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	return st
}

func phpFPMVersions() []string {
	var found []string
	for _, v := range []string{"8.1", "8.2", "8.3", "8.4"} {
		if ok, _ := dpkgVersion("php" + v + "-fpm"); ok {
			found = append(found, v)
		}
	}
	return found
}

func enablePHPFPMStatus() error {
	files, _ := filepath.Glob("/etc/php/*/fpm/pool.d/*.conf")
	reload := map[string]struct{}{}
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		next := ensurePoolStatusPath(string(b))
		if next == string(b) {
			continue
		}
		if os.WriteFile(path, []byte(next), 0644) != nil {
			continue
		}
		if ver := phpVerFromPath(path); ver != "" {
			reload[ver] = struct{}{}
		}
	}
	for ver := range reload {
		_ = exec.Command("systemctl", "reload", "php"+ver+"-fpm").Run()
	}
	return nil
}

func phpVerFromPath(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for i, p := range parts {
		if p == "php" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func ensurePoolStatusPath(content string) string {
	var b strings.Builder
	found := false
	for _, raw := range strings.SplitAfter(content, "\n") {
		trim := strings.TrimSpace(raw)
		if strings.HasPrefix(strings.TrimLeft(trim, ";"), "pm.status_path") {
			if !found {
				b.WriteString("pm.status_path = /fpm-status\n")
				found = true
			}
			continue
		}
		b.WriteString(raw)
	}
	out := b.String()
	if !found {
		if !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		out += "pm.status_path = /fpm-status\n"
	}
	return out
}

func listFPMPools() []rpc.FPMPool {
	files, _ := filepath.Glob("/etc/php/*/fpm/pool.d/*.conf")
	var out []rpc.FPMPool
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		p := parsePoolFile(string(b))
		p.Version = phpVerFromPath(path)
		if p.Name == "" {
			p.Name = strings.TrimSuffix(filepath.Base(path), ".conf")
		}
		out = append(out, p)
	}
	return out
}

func parsePoolFile(content string) rpc.FPMPool {
	p := rpc.FPMPool{}
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") && p.Name == "" {
			p.Name = strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "listen":
			p.Listen = v
		case "pm":
			p.PM = v
		}
	}
	return p
}

type fpmJSON struct {
	Pool               string `json:"pool"`
	ProcessManager     string `json:"process manager"`
	StartSince         int64  `json:"start since"`
	AcceptedConn       int64  `json:"accepted conn"`
	ListenQueue        int    `json:"listen queue"`
	MaxListenQueue     int    `json:"max listen queue"`
	IdleProcesses      int    `json:"idle processes"`
	ActiveProcesses    int    `json:"active processes"`
	TotalProcesses     int    `json:"total processes"`
	MaxActiveProcesses int    `json:"max active processes"`
	MaxChildrenReached int    `json:"max children reached"`
	SlowRequests       int64  `json:"slow requests"`
	Processes          []struct {
		PID             int    `json:"pid"`
		State           string `json:"state"`
		RequestMethod   string `json:"request method"`
		RequestURI      string `json:"request uri"`
		User            string `json:"user"`
		Script          string `json:"script"`
		RequestDuration int64  `json:"request duration"`
	} `json:"processes"`
}

func parseFPMJSON(p *rpc.FPMPool, raw []byte) error {
	raw = stripCGI(raw)
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		msg := strings.TrimSpace(string(raw))
		if len(msg) > 180 {
			msg = msg[:180]
		}
		if msg == "" {
			msg = "empty FPM status"
		}
		return fmt.Errorf("%s", msg)
	}
	var j fpmJSON
	if err := json.Unmarshal(raw, &j); err != nil {
		return fmt.Errorf("status json: %w", err)
	}
	if j.Pool != "" {
		p.Name = j.Pool
	}
	if j.ProcessManager != "" {
		p.PM = j.ProcessManager
	}
	p.StartSince = j.StartSince
	p.Accepted = j.AcceptedConn
	p.ListenQueue = j.ListenQueue
	p.MaxListenQueue = j.MaxListenQueue
	p.Idle = j.IdleProcesses
	p.Active = j.ActiveProcesses
	p.Total = j.TotalProcesses
	p.MaxActive = j.MaxActiveProcesses
	p.MaxChildrenReached = j.MaxChildrenReached
	p.SlowRequests = j.SlowRequests
	for _, pr := range j.Processes {
		if strings.Contains(pr.RequestURI, "/fpm-status") {
			continue
		}
		if !strings.EqualFold(pr.State, "Running") && pr.RequestURI == "" && pr.Script == "" {
			continue
		}
		p.Processes = append(p.Processes, rpc.FPMProcess{
			PID:      pr.PID,
			State:    pr.State,
			Method:   pr.RequestMethod,
			URI:      pr.RequestURI,
			User:     pr.User,
			Script:   pr.Script,
			Duration: pr.RequestDuration,
		})
		if len(p.Processes) >= 40 {
			break
		}
	}
	return nil
}

func fpmQuery(listen, path string) ([]byte, error) {
	network, addr := "unix", listen
	if strings.Contains(listen, ":") && !strings.HasPrefix(listen, "/") {
		network = "tcp"
		addr = listen
	}
	conn, err := net.DialTimeout(network, addr, 2*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	params := map[string]string{
		"SCRIPT_NAME":        path,
		"SCRIPT_FILENAME":    path,
		"QUERY_STRING":       "json&full",
		"REQUEST_METHOD":     "GET",
		"REQUEST_URI":        path + "?json&full",
		"DOCUMENT_URI":       path,
		"SERVER_SOFTWARE":    "siroc",
		"SERVER_PROTOCOL":    "HTTP/1.0",
		"GATEWAY_INTERFACE":  "CGI/1.1",
		"REMOTE_ADDR":        "127.0.0.1",
		"SERVER_NAME":        "localhost",
		"SERVER_PORT":        "80",
	}
	if err := fcgiBegin(conn, 1); err != nil {
		return nil, err
	}
	if err := writeFCGIParams(conn, 1, params); err != nil {
		return nil, err
	}
	if err := writeFCGI(conn, 5, 1, nil); err != nil { // STDIN empty
		return nil, err
	}
	return readFCGIStdout(conn, 1)
}

const fcgiRecBegin = 1
const fcgiRecParams = 4

func fcgiBegin(w io.Writer, req uint16) error {
	body := []byte{0, 1, 0, 0, 0, 0, 0, 0} // responder
	return writeFCGI(w, fcgiRecBegin, req, body)
}

func writeFCGIParams(w io.Writer, req uint16, kv map[string]string) error {
	var buf bytes.Buffer
	for k, v := range kv {
		kb, vb := []byte(k), []byte(v)
		writeNVLen(&buf, len(kb))
		writeNVLen(&buf, len(vb))
		buf.Write(kb)
		buf.Write(vb)
	}
	if err := writeFCGI(w, fcgiRecParams, req, buf.Bytes()); err != nil {
		return err
	}
	return writeFCGI(w, fcgiRecParams, req, nil)
}

func writeNVLen(buf *bytes.Buffer, n int) {
	if n < 128 {
		buf.WriteByte(byte(n))
		return
	}
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(n)|0x80000000)
	buf.Write(b[:])
}

func writeFCGI(w io.Writer, typ byte, req uint16, content []byte) error {
	pad := (8 - (len(content) % 8)) % 8
	hdr := []byte{1, typ, byte(req >> 8), byte(req), byte(len(content) >> 8), byte(len(content)), byte(pad), 0}
	if _, err := w.Write(hdr); err != nil {
		return err
	}
	if len(content) > 0 {
		if _, err := w.Write(content); err != nil {
			return err
		}
	}
	if pad > 0 {
		if _, err := w.Write(make([]byte, pad)); err != nil {
			return err
		}
	}
	return nil
}

func readFCGIStdout(r io.Reader, req uint16) ([]byte, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	hdr := make([]byte, 8)
	for {
		if _, err := io.ReadFull(r, hdr); err != nil {
			if stdout.Len() > 0 {
				break
			}
			return nil, err
		}
		typ := hdr[1]
		id := binary.BigEndian.Uint16(hdr[2:4])
		clen := int(binary.BigEndian.Uint16(hdr[4:6]))
		pad := int(hdr[6])
		body := make([]byte, clen+pad)
		if clen+pad > 0 {
			if _, err := io.ReadFull(r, body); err != nil {
				return nil, err
			}
		}
		content := body[:clen]
		if id != req {
			continue
		}
		switch typ {
		case 6:
			stdout.Write(content)
		case 7:
			stderr.Write(content)
		case 3:
			if stdout.Len() == 0 {
				msg := strings.TrimSpace(stderr.String())
				if msg == "" {
					msg = "empty FPM status"
				}
				return nil, fmt.Errorf("%s", msg)
			}
			return stdout.Bytes(), nil
		}
	}
	if stdout.Len() == 0 {
		return nil, fmt.Errorf("empty FPM status")
	}
	return stdout.Bytes(), nil
}

func stripCGI(raw []byte) []byte {
	s := string(raw)
	if i := strings.Index(s, "\r\n\r\n"); i >= 0 {
		return []byte(s[i+4:])
	}
	if i := strings.Index(s, "\n\n"); i >= 0 {
		return []byte(s[i+2:])
	}
	return raw
}

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
	"strconv"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

const apacheStatusConf = `/etc/apache2/conf-available/siroc-status.conf`
const nginxStatusDeny = `/etc/nginx/snippets/siroc-deny-status.conf`

func ApacheStatus() *rpc.ApacheStatus {
	st := &rpc.ApacheStatus{
		ScoreLegend: scoreLegend(),
	}
	ok, ver := dpkgVersion("apache2")
	st.Installed = ok
	st.Version = ver
	if !ok {
		st.Message = "Apache is not installed. Install it from Software."
		return st
	}
	st.Active = serviceActive("apache2")
	_ = enableApacheStatus()
	if !st.Active {
		st.Message = "Apache is installed but not running."
		return st
	}
	raw, err := fetchApacheStatus("?auto")
	if err != nil {
		st.Message = "mod_status is not reachable yet. " + err.Error()
		return st
	}
	parseApacheAuto(st, raw)
	if html, err := fetchApacheStatus(""); err == nil {
		st.Workers = parseApacheWorkers(html)
	}
	st.VHosts = apacheVHosts()
	st.Ready = true
	st.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	return st
}

func serviceUptimeSec(name string) int64 {
	out, err := exec.Command("systemctl", "show", name, "--property=ActiveEnterTimestampUnix", "--value").Output()
	if err == nil {
		if ts := atoi64(strings.TrimSpace(string(out))); ts > 0 {
			n := time.Now().Unix() - ts
			if n > 0 {
				return n
			}
		}
	}
	out, err = exec.Command("systemctl", "show", name, "--property=ActiveEnterTimestamp", "--value").Output()
	if err != nil {
		return 0
	}
	s := strings.TrimSpace(string(out))
	if s == "" || strings.EqualFold(s, "n/a") {
		return 0
	}
	for _, layout := range []string{
		"Mon 2006-01-02 15:04:05 MST",
		"Mon 2006-01-02 15:04:05 UTC",
		time.RFC3339,
	} {
		t, e := time.Parse(layout, s)
		if e != nil {
			continue
		}
		n := time.Now().Unix() - t.Unix()
		if n > 0 {
			return n
		}
	}
	return 0
}

func enableApacheStatus() error {
	if _, err := exec.LookPath("a2enmod"); err != nil {
		return fmt.Errorf("Apache is not installed")
	}
	_ = os.MkdirAll("/etc/apache2/conf-available", 0755)
	_ = os.MkdirAll("/etc/nginx/snippets", 0755)
	body := `<IfModule mod_status.c>
    ExtendedStatus On
    <Location /server-status>
        SetHandler server-status
        Require ip 127.0.0.1 ::1
    </Location>
</IfModule>
`
	changed := writeIfChanged(apacheStatusConf, body)
	_ = writeIfChanged(nginxStatusDeny, "location ^~ /server-status { return 404; }\nlocation ^~ /nginx-status { return 404; }\nlocation ^~ /fpm-status { return 404; }\n")
	_ = exec.Command("a2enmod", "status").Run()
	_ = exec.Command("a2enconf", "siroc-status").Run()
	denyPublicServerStatus()
	if changed {
		_ = exec.Command("apache2ctl", "configtest").Run()
		_ = exec.Command("systemctl", "reload", "apache2").Run()
	}
	return nil
}

func writeIfChanged(path, body string) bool {
	old, _ := os.ReadFile(path)
	if string(old) == body {
		return false
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		return false
	}
	return true
}

func denyPublicServerStatus() {
	paths, _ := filepath.Glob("/etc/nginx/sites-available/*.conf")
	paths = append(paths, "/etc/nginx/sites-enabled/default")
	changed := false
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		s := string(b)
		if strings.Contains(s, "siroc-deny-status.conf") || strings.Contains(s, "location ^~ /server-status") {
			continue
		}
		if !strings.Contains(s, "server {") {
			continue
		}
		next := strings.ReplaceAll(s, "server {", "server {\n    include /etc/nginx/snippets/siroc-deny-status.conf;")
		if next != s {
			if os.WriteFile(p, []byte(next), 0644) == nil {
				changed = true
			}
		}
	}
	if changed {
		_ = exec.Command("nginx", "-t").Run()
		_ = exec.Command("systemctl", "reload", "nginx").Run()
	}
}

func fetchApacheStatus(query string) (string, error) {
	cli := &http.Client{Timeout: 2 * time.Second}
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:8080/server-status"+query, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "siroc")
	res, err := cli.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if res.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return string(b), nil
}

func parseApacheAuto(st *rpc.ApacheStatus, raw string) {
	for _, line := range strings.Split(raw, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), ": ")
		if !ok {
			if strings.HasPrefix(line, "Scoreboard:") {
				st.Scoreboard = strings.TrimSpace(strings.TrimPrefix(line, "Scoreboard:"))
			}
			continue
		}
		switch k {
		case "ServerVersion":
			st.ServerVersion = v
		case "ServerMPM":
			st.MPM = v
		case "ServerBuilt":
			st.Built = v
		case "CurrentTime":
			st.CurrentTime = v
		case "RestartTime":
			st.RestartTime = v
		case "ParentServerConfigGeneration":
			st.ConfigGen = atoi(v)
		case "Uptime":
			st.UptimeSec = atoi64(v)
		case "Total Accesses":
			st.TotalAccesses = atoi64(v)
		case "Total kBytes":
			st.TotalBytes = atoi64(v) * 1024
		case "CPULoad":
			st.CPULoad = atof(v)
		case "ReqPerSec":
			st.ReqPerSec = atof(v)
		case "BytesPerSec":
			st.BytesPerSec = atof(v)
		case "BytesPerReq":
			st.BytesPerReq = atof(v)
		case "BusyWorkers":
			st.BusyWorkers = atoi(v)
		case "IdleWorkers":
			st.IdleWorkers = atoi(v)
		case "ConnsTotal":
			st.ConnsTotal = atoi(v)
		case "ConnsAsyncWriting":
			st.ConnsWriting = atoi(v)
		case "ConnsAsyncKeepAlive":
			st.ConnsKeepAlive = atoi(v)
		case "ConnsAsyncClosing":
			st.ConnsClosing = atoi(v)
		case "Load1":
			st.Load1 = atof(v)
		case "Load5":
			st.Load5 = atof(v)
		case "Load15":
			st.Load15 = atof(v)
		case "Scoreboard":
			st.Scoreboard = v
		}
	}
	st.TotalWorkers = st.BusyWorkers + st.IdleWorkers
	if n := len(strings.ReplaceAll(st.Scoreboard, " ", "")); n > st.TotalWorkers {
		st.TotalWorkers = n
	}
	if st.ServerVersion != "" {
		st.Version = st.ServerVersion
	}
	st.ScoreCounts = countScore(st.Scoreboard)
	if st.TotalWorkers > 0 {
		st.BusyPercent = float64(st.BusyWorkers) * 100 / float64(st.TotalWorkers)
	}
}

func countScore(board string) []rpc.ApacheScoreCount {
	order := []string{"W", "K", "R", "S", "D", "C", "L", "G", "I", "_", "."}
	seen := map[string]int{}
	for _, ch := range board {
		c := string(ch)
		if c == " " || c == "\n" {
			continue
		}
		seen[c]++
	}
	var out []rpc.ApacheScoreCount
	used := map[string]bool{}
	for _, k := range order {
		if n := seen[k]; n > 0 {
			out = append(out, rpc.ApacheScoreCount{Key: k, Count: n, Title: scoreTitle(k)})
			used[k] = true
		}
	}
	for k, n := range seen {
		if !used[k] {
			out = append(out, rpc.ApacheScoreCount{Key: k, Count: n, Title: scoreTitle(k)})
		}
	}
	return out
}

func scoreTitle(k string) string {
	switch k {
	case "_":
		return "Waiting"
	case "S":
		return "Starting"
	case "R":
		return "Reading"
	case "W":
		return "Sending"
	case "K":
		return "Keepalive"
	case "D":
		return "DNS lookup"
	case "C":
		return "Closing"
	case "L":
		return "Logging"
	case "G":
		return "Graceful"
	case "I":
		return "Idle cleanup"
	case ".":
		return "Open slot"
	default:
		return k
	}
}

func scoreLegend() []rpc.ApacheScoreCount {
	keys := []string{"W", "K", "R", "S", "_", ".", "C", "G", "D", "L", "I"}
	out := make([]rpc.ApacheScoreCount, 0, len(keys))
	for _, k := range keys {
		out = append(out, rpc.ApacheScoreCount{Key: k, Title: scoreTitle(k)})
	}
	return out
}

var tdRe = regexp.MustCompile(`(?is)<td[^>]*>(.*?)</td>`)
var trRe = regexp.MustCompile(`(?is)<tr>(.*?)</tr>`)
var tagRe = regexp.MustCompile(`(?is)<[^>]+>`)

func parseApacheWorkers(html string) []rpc.ApacheWorker {
	var out []rpc.ApacheWorker
	for _, m := range trRe.FindAllStringSubmatch(html, 400) {
		cells := tdRe.FindAllStringSubmatch(m[1], -1)
		if len(cells) < 14 {
			continue
		}
		vals := make([]string, len(cells))
		for i, c := range cells {
			vals[i] = strings.TrimSpace(tagRe.ReplaceAllString(c[1], ""))
		}
		if vals[0] == "Sum" || vals[2] == "no" {
			continue
		}
		state := vals[3]
		if len(state) != 1 || !strings.ContainsAny(state, "SRWKDCLGI") {
			continue
		}
		w := rpc.ApacheWorker{
			Srv:   vals[0],
			PID:   atoi(vals[1]),
			Acc:   vals[2],
			State: state,
			Label: scoreTitle(state),
			CPU:   atof(vals[4]),
			SS:    atoi(vals[5]),
			Req:   atoi(vals[6]),
		}
		switch {
		case len(vals) >= 15:
			w.Client, w.Protocol, w.VHost, w.Request = vals[11], vals[12], vals[13], vals[14]
		case len(vals) >= 14:
			w.Client, w.Protocol, w.VHost, w.Request = vals[10], vals[11], vals[12], vals[13]
		default:
			w.Client, w.VHost, w.Request = vals[10], vals[11], vals[12]
		}
		out = append(out, w)
		if len(out) >= 80 {
			break
		}
	}
	return out
}

func apacheVHosts() []rpc.ApacheVHost {
	outb, err := exec.Command("apache2ctl", "-S").CombinedOutput()
	if err != nil {
		outb, err = exec.Command("apachectl", "-S").CombinedOutput()
		if err != nil {
			return nil
		}
	}
	var list []rpc.ApacheVHost
	re := regexp.MustCompile(`(?m)^\s*(?:port\s+(\d+)\s+)?namevhost\s+(\S+)\s+\(([^)]+)\)`)
	for _, m := range re.FindAllStringSubmatch(string(outb), -1) {
		port := 8080
		if m[1] != "" {
			port = atoi(m[1])
		}
		list = append(list, rpc.ApacheVHost{
			Name: m[2],
			Port: port,
			File: m[3],
		})
	}
	return list
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func atoi64(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}

func atof(s string) float64 {
	n, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return n
}

//go:build linux

package software

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
)

const (
	nginxConfPath      = "/etc/nginx/nginx.conf"
	nginxOptimizePath  = "/etc/nginx/conf.d/99-cp-optimize.conf"
	apacheOptimizePath = "/etc/apache2/conf-available/zz-cp-optimize.conf"
	apacheMPMPath      = "/etc/apache2/mods-available/mpm_event.conf"
	webOptimizeState   = "/var/lib/siroc/weboptimize.json"
)

var webValRe = regexp.MustCompile(`^[A-Za-z0-9._:=+*/(), \-]{1,800}$`)

type webKnob struct {
	Name  string
	Label string
	Group string
}

func webKnobs() []webKnob {
	return []webKnob{
		{Name: "worker_processes", Label: "Worker processes", Group: "nginx"},
		{Name: "worker_connections", Label: "Worker connections", Group: "nginx"},
		{Name: "worker_rlimit_nofile", Label: "Open files limit", Group: "nginx"},
		{Name: "multi_accept", Label: "Multi accept", Group: "nginx"},
		{Name: "keepalive_timeout", Label: "Keepalive timeout", Group: "nginx"},
		{Name: "keepalive_requests", Label: "Keepalive requests", Group: "nginx"},
		{Name: "gzip", Label: "Gzip", Group: "nginx"},
		{Name: "gzip_comp_level", Label: "Gzip level", Group: "nginx"},
		{Name: "open_file_cache", Label: "Open file cache", Group: "nginx"},
		{Name: "sendfile", Label: "Sendfile", Group: "nginx"},
		{Name: "tcp_nopush", Label: "TCP nopush", Group: "nginx"},
		{Name: "tcp_nodelay", Label: "TCP nodelay", Group: "nginx"},
		{Name: "server_tokens", Label: "Server tokens", Group: "nginx"},
		{Name: "server_names_hash_bucket_size", Label: "Server names hash bucket", Group: "nginx"},
		{Name: "proxy_read_timeout", Label: "Proxy read timeout", Group: "nginx"},
		{Name: "ssl_protocols", Label: "SSL protocols", Group: "nginx"},
		{Name: "Timeout", Label: "Timeout", Group: "apache"},
		{Name: "KeepAlive", Label: "KeepAlive", Group: "apache"},
		{Name: "KeepAliveTimeout", Label: "KeepAlive timeout", Group: "apache"},
		{Name: "MaxKeepAliveRequests", Label: "Max keepalive requests", Group: "apache"},
		{Name: "HostnameLookups", Label: "Hostname lookups", Group: "apache"},
		{Name: "ServerTokens", Label: "Server tokens", Group: "apache"},
		{Name: "StartServers", Label: "Start servers", Group: "apache"},
		{Name: "ThreadsPerChild", Label: "Threads per child", Group: "apache"},
		{Name: "MaxRequestWorkers", Label: "Max request workers", Group: "apache"},
		{Name: "MaxConnectionsPerChild", Label: "Max connections per child", Group: "apache"},
	}
}

func WebOptimize(ramGB int) *rpc.WebOptimize {
	st := &rpc.WebOptimize{CPUs: runtime.NumCPU()}
	st.TotalRAM = webMemTotal()
	st.TotalRAMGB = roundRAMGB(st.TotalRAM)
	if st.TotalRAMGB < 1 {
		st.TotalRAMGB = 1
	}
	st.SuggestedGB = clampInt(st.TotalRAMGB, 1, 128)
	if ramGB <= 0 {
		if n := readSavedWebRAM(); n > 0 {
			ramGB = n
		} else {
			ramGB = st.SuggestedGB
		}
	}
	st.RAMGB = clampInt(ramGB, 1, 128)
	ngxOK, _ := dpkgVersion("nginx")
	if !ngxOK {
		if _, err := exec.LookPath("nginx"); err == nil {
			ngxOK = true
		}
	}
	st.NginxInstalled = ngxOK
	st.NginxActive = serviceActive("nginx")
	apOK, _ := dpkgVersion("apache2")
	if !apOK {
		if _, err := os.Stat("/etc/apache2/apache2.conf"); err == nil {
			apOK = true
		}
	}
	st.ApacheInstalled = apOK
	st.ApacheActive = serviceActive("apache2")
	if !ngxOK && !apOK {
		st.Message = "Install Nginx or Apache from Software first."
		return st
	}
	rec := webTune(st.RAMGB, st.CPUs)
	live := mergeWebLive()
	for _, k := range webKnobs() {
		if k.Group == "nginx" && !ngxOK {
			continue
		}
		if k.Group == "apache" && !apOK {
			continue
		}
		item := rpc.WebOptimizeItem{Name: k.Name, Label: k.Label, Group: k.Group, Recommend: rec[k.Name]}
		if v := live[k.Name]; v != "" {
			item.Live = v
			item.Value = rec[k.Name]
			if rec[k.Name] == "" {
				item.Value = v
			}
		} else if rec[k.Name] != "" {
			item.Value = rec[k.Name]
		}
		st.Settings = append(st.Settings, item)
	}
	return st
}

func ApplyWebOptimize(in rpc.WebOptimizeApply) error {
	st := WebOptimize(in.RAMGB)
	if !st.NginxInstalled && !st.ApacheInstalled {
		return fmt.Errorf("install Nginx or Apache first")
	}
	vals := webTune(st.RAMGB, st.CPUs)
	for k, v := range in.Settings {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if !webValRe.MatchString(v) {
			return fmt.Errorf("invalid value for %s", k)
		}
		vals[k] = v
	}
	if st.NginxInstalled {
		if err := applyNginxOptimize(vals); err != nil {
			return err
		}
	}
	if st.ApacheInstalled {
		if err := applyApacheOptimize(vals); err != nil {
			return err
		}
	}
	_ = os.MkdirAll(filepath.Dir(webOptimizeState), 0750)
	_ = os.WriteFile(webOptimizeState, []byte(`{"ramGB":`+strconv.Itoa(st.RAMGB)+`}`+"\n"), 0640)
	if st.NginxInstalled {
		if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
			return fmt.Errorf("nginx -t: %s", strings.TrimSpace(string(out)))
		}
		if out, err := exec.Command("systemctl", "restart", "nginx").CombinedOutput(); err != nil {
			return fmt.Errorf("restart nginx: %s", strings.TrimSpace(string(out)))
		}
	}
	if st.ApacheInstalled {
		if out, err := exec.Command("apache2ctl", "configtest").CombinedOutput(); err != nil {
			return fmt.Errorf("apache configtest: %s", strings.TrimSpace(string(out)))
		}
		if out, err := exec.Command("systemctl", "restart", "apache2").CombinedOutput(); err != nil {
			return fmt.Errorf("restart apache2: %s", strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func webTune(ramGB, cpus int) map[string]string {
	if ramGB < 1 {
		ramGB = 1
	}
	if cpus < 1 {
		cpus = 1
	}
	workers := ramGB
	if workers > cpus {
		workers = cpus
	}
	if workers > 8 {
		workers = 8
	}
	if workers < 1 {
		workers = 1
	}
	conns := 1024
	switch {
	case ramGB >= 16:
		conns = 16384
	case ramGB >= 8:
		conns = 8192
	case ramGB >= 4:
		conns = 4096
	case ramGB >= 2:
		conns = 2048
	}
	nofile := conns * 2
	if nofile < 2048 {
		nofile = 2048
	}
	apacheWorkers := 50
	switch {
	case ramGB >= 16:
		apacheWorkers = 400
	case ramGB >= 8:
		apacheWorkers = 250
	case ramGB >= 4:
		apacheWorkers = 150
	case ramGB >= 2:
		apacheWorkers = 75
	}
	threads := 25
	apacheWorkers = (apacheWorkers / threads) * threads
	if apacheWorkers < threads {
		apacheWorkers = threads
	}
	start := 2
	if ramGB >= 8 {
		start = 3
	}
	return map[string]string{
		"worker_processes":              strconv.Itoa(workers),
		"worker_connections":            strconv.Itoa(conns),
		"worker_rlimit_nofile":          strconv.Itoa(nofile),
		"multi_accept":                  "on",
		"keepalive_timeout":             "65",
		"keepalive_requests":            "1000",
		"gzip":                          "on",
		"gzip_comp_level":               "5",
		"open_file_cache":               "max=10000 inactive=20s",
		"sendfile":                      "on",
		"tcp_nopush":                    "on",
		"tcp_nodelay":                   "on",
		"server_tokens":                 "off",
		"server_names_hash_bucket_size": "128",
		"proxy_read_timeout":            "300s",
		"ssl_protocols":                 "TLSv1.2 TLSv1.3",
		"Timeout":                       "60",
		"KeepAlive":                     "Off",
		"KeepAliveTimeout":              "2",
		"MaxKeepAliveRequests":          "100",
		"HostnameLookups":               "Off",
		"ServerTokens":                  "Prod",
		"StartServers":                  strconv.Itoa(start),
		"ThreadsPerChild":               strconv.Itoa(threads),
		"MaxRequestWorkers":             strconv.Itoa(apacheWorkers),
		"MaxConnectionsPerChild":        "10000",
	}
}

func applyNginxOptimize(vals map[string]string) error {
	b, err := os.ReadFile(nginxConfPath)
	if err != nil {
		return fmt.Errorf("read nginx.conf: %w", err)
	}
	if _, err := os.Stat(nginxConfPath + ".cp-bak"); err != nil {
		_ = exec.Command("cp", "-a", nginxConfPath, nginxConfPath+".cp-bak").Run()
	}
	conf := commentNginxHTTP(string(b), nginxHTTPDupKeys)
	conf = setNginxStmt(conf, "worker_processes", vals["worker_processes"])
	conf = setNginxStmt(conf, "worker_connections", vals["worker_connections"])
	if regexp.MustCompile(`(?m)^\s*worker_rlimit_nofile\s+`).MatchString(conf) {
		conf = setNginxStmt(conf, "worker_rlimit_nofile", vals["worker_rlimit_nofile"])
	} else {
		conf = insertAfterNginx(conf, "worker_processes", "worker_rlimit_nofile  "+vals["worker_rlimit_nofile"]+";")
	}
	if regexp.MustCompile(`(?m)^\s*multi_accept\s+`).MatchString(conf) {
		conf = setNginxStmt(conf, "multi_accept", vals["multi_accept"])
	} else {
		conf = insertAfterNginx(conf, "worker_connections", "    multi_accept  "+vals["multi_accept"]+";")
	}
	if err := os.WriteFile(nginxConfPath, []byte(conf), 0644); err != nil {
		return err
	}
	if err := os.MkdirAll("/etc/nginx/conf.d", 0755); err != nil {
		return err
	}
	body := nginxOptimizeConf(vals)
	return os.WriteFile(nginxOptimizePath, []byte(body), 0644)
}

var nginxHTTPDupKeys = []string{
	"sendfile", "tcp_nopush", "tcp_nodelay",
	"keepalive_timeout", "keepalive_requests",
	"server_tokens", "server_names_hash_bucket_size", "types_hash_max_size",
	"client_body_buffer_size", "client_header_buffer_size", "large_client_header_buffers",
	"reset_timedout_connection",
	"gzip", "gzip_vary", "gzip_proxied", "gzip_comp_level", "gzip_min_length", "gzip_types",
	"open_file_cache", "open_file_cache_valid", "open_file_cache_min_uses", "open_file_cache_errors",
	"proxy_buffer_size", "proxy_buffers", "proxy_busy_buffers_size",
	"proxy_connect_timeout", "proxy_send_timeout", "proxy_read_timeout",
	"ssl_session_cache", "ssl_session_timeout", "ssl_protocols",
}

func commentNginxHTTP(conf string, keys []string) string {
	loc := regexp.MustCompile(`(?m)^[ \t]*http[ \t]*\{`).FindStringIndex(conf)
	if loc == nil {
		return conf
	}
	open := strings.LastIndex(conf[:loc[1]], "{")
	if open < 0 {
		return conf
	}
	depth := 1
	end := -1
	for i := open + 1; i < len(conf); i++ {
		switch conf[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i
				i = len(conf)
			}
		}
	}
	if end < 0 {
		return conf
	}
	set := map[string]struct{}{}
	for _, k := range keys {
		set[k] = struct{}{}
	}
	block := conf[open+1 : end]
	lines := strings.Split(block, "\n")
	depth = 1
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if depth == 1 && trim != "" && !strings.HasPrefix(trim, "#") {
			fields := strings.Fields(trim)
			if len(fields) > 0 {
				key := strings.TrimSuffix(fields[0], ";")
				if _, ok := set[key]; ok {
					wsLen := len(line) - len(strings.TrimLeft(line, " \t"))
					lines[i] = line[:wsLen] + "# " + strings.TrimSpace(line)
				}
			}
		}
		depth += strings.Count(line, "{") - strings.Count(line, "}")
	}
	return conf[:open+1] + strings.Join(lines, "\n") + conf[end:]
}

func nginxOptimizeConf(v map[string]string) string {
	return `# Siroc web optimize
sendfile ` + v["sendfile"] + `;
tcp_nopush ` + v["tcp_nopush"] + `;
tcp_nodelay ` + v["tcp_nodelay"] + `;
keepalive_timeout ` + v["keepalive_timeout"] + `;
keepalive_requests ` + v["keepalive_requests"] + `;
server_tokens ` + v["server_tokens"] + `;
server_names_hash_bucket_size ` + v["server_names_hash_bucket_size"] + `;
types_hash_max_size 4096;
client_body_buffer_size 128k;
client_header_buffer_size 1k;
large_client_header_buffers 4 32k;
reset_timedout_connection on;
gzip ` + v["gzip"] + `;
gzip_vary on;
gzip_proxied any;
gzip_comp_level ` + v["gzip_comp_level"] + `;
gzip_min_length 256;
gzip_types text/plain text/css text/xml text/javascript application/json application/javascript application/xml application/rss+xml application/atom+xml image/svg+xml;
open_file_cache ` + v["open_file_cache"] + `;
open_file_cache_valid 30s;
open_file_cache_min_uses 2;
open_file_cache_errors on;
proxy_buffer_size 16k;
proxy_buffers 8 16k;
proxy_busy_buffers_size 32k;
proxy_connect_timeout 60s;
proxy_send_timeout 300s;
proxy_read_timeout ` + v["proxy_read_timeout"] + `;
ssl_session_cache shared:SSL:10m;
ssl_session_timeout 1d;
ssl_protocols ` + v["ssl_protocols"] + `;
`
}

func applyApacheOptimize(vals map[string]string) error {
	if err := os.MkdirAll("/etc/apache2/conf-available", 0755); err != nil {
		return err
	}
	body := `# Siroc web optimize
Timeout ` + vals["Timeout"] + `
KeepAlive ` + vals["KeepAlive"] + `
MaxKeepAliveRequests ` + vals["MaxKeepAliveRequests"] + `
KeepAliveTimeout ` + vals["KeepAliveTimeout"] + `
HostnameLookups ` + vals["HostnameLookups"] + `
ServerTokens ` + vals["ServerTokens"] + `
ServerSignature Off
`
	if err := os.WriteFile(apacheOptimizePath, []byte(body), 0644); err != nil {
		return err
	}
	_ = exec.Command("a2enconf", "zz-cp-optimize").Run()
	mpm := fmt.Sprintf(`# Siroc optimized event MPM
<IfModule mpm_event_module>
	StartServers            %s
	MinSpareThreads         25
	MaxSpareThreads         75
	ThreadLimit             64
	ThreadsPerChild         %s
	MaxRequestWorkers       %s
	MaxConnectionsPerChild  %s
</IfModule>
`, vals["StartServers"], vals["ThreadsPerChild"], vals["MaxRequestWorkers"], vals["MaxConnectionsPerChild"])
	if _, err := os.Stat(apacheMPMPath); err == nil {
		if _, err := os.Stat(apacheMPMPath + ".cp-bak"); err != nil {
			_ = exec.Command("cp", "-a", apacheMPMPath, apacheMPMPath+".cp-bak").Run()
		}
		if err := os.WriteFile(apacheMPMPath, []byte(mpm), 0644); err != nil {
			return err
		}
	}
	_ = exec.Command("a2dismod", "-f", "mpm_prefork").Run()
	_ = exec.Command("a2dismod", "-f", "mpm_worker").Run()
	_ = exec.Command("a2enmod", "mpm_event").Run()
	return nil
}

func mergeWebLive() map[string]string {
	out := map[string]string{}
	readWebFile(nginxConfPath, out)
	readWebFile(nginxOptimizePath, out)
	readWebFile("/etc/apache2/apache2.conf", out)
	readWebFile("/etc/apache2/conf-enabled/security.conf", out)
	readWebFile(apacheMPMPath, out)
	readWebFile(apacheOptimizePath, out)
	readWebFile("/etc/apache2/conf-enabled/zz-cp-optimize.conf", out)
	return out
}

func readWebFile(path string, out map[string]string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	keys := map[string]struct{}{}
	for _, k := range webKnobs() {
		keys[k.Name] = struct{}{}
	}
	for _, line := range strings.Split(string(b), "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		t = strings.TrimSuffix(t, ";")
		fields := strings.Fields(t)
		if len(fields) < 2 {
			continue
		}
		if _, ok := keys[fields[0]]; ok {
			out[fields[0]] = strings.Join(fields[1:], " ")
		}
	}
}

func setNginxStmt(conf, key, val string) string {
	re := regexp.MustCompile(`(?m)^([ \t]*)` + regexp.QuoteMeta(key) + `[ \t]+[^;\n]*;`)
	repl := "${1}" + key + "  " + val + ";"
	loc := re.FindStringIndex(conf)
	if loc == nil {
		return conf
	}
	return conf[:loc[0]] + re.ReplaceAllString(conf[loc[0]:loc[1]], repl) + conf[loc[1]:]
}

func insertAfterNginx(conf, afterKey, stmt string) string {
	re := regexp.MustCompile(`(?m)^[ \t]*` + regexp.QuoteMeta(afterKey) + `[ \t]+[^;\n]*;[ \t]*\n`)
	loc := re.FindStringIndex(conf)
	if loc == nil {
		return conf
	}
	return conf[:loc[1]] + stmt + "\n" + conf[loc[1]:]
}

func readSavedWebRAM() int {
	b, err := os.ReadFile(webOptimizeState)
	if err != nil {
		return 0
	}
	re := regexp.MustCompile(`"ramGB"\s*:\s*(\d+)`)
	m := re.FindStringSubmatch(string(b))
	if len(m) != 2 {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

func webMemTotal() int64 {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0
		}
		kb, _ := strconv.ParseInt(fields[1], 10, 64)
		return kb * 1024
	}
	return 0
}

func clampInt(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}

func roundRAMGB(total int64) int {
	if total <= 0 {
		return 1
	}
	const gb = int64(1024 * 1024 * 1024)
	n := int((total + gb/2) / gb)
	if n < 1 {
		return 1
	}
	return n
}

//go:build linux

package dbmgmt

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

const sirocMySQLConfName = "99-siroc.cnf"

var dbValueRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type dbKnob struct {
	Name  string
	Label string
}

func dbKnobs(mariadb bool) []dbKnob {
	out := []dbKnob{
		{Name: "bind_address", Label: "Bind address"},
		{Name: "innodb_buffer_pool_size", Label: "InnoDB buffer pool"},
		{Name: "innodb_buffer_pool_instances", Label: "Buffer pool instances"},
		{Name: "innodb_log_buffer_size", Label: "InnoDB log buffer"},
	}
	if mariadb {
		out = append(out, dbKnob{Name: "innodb_log_file_size", Label: "InnoDB log file"})
	} else {
		out = append(out, dbKnob{Name: "innodb_redo_log_capacity", Label: "InnoDB redo log"})
	}
	out = append(out,
		dbKnob{Name: "innodb_flush_log_at_trx_commit", Label: "Flush log at commit"},
		dbKnob{Name: "innodb_flush_method", Label: "InnoDB flush method"},
		dbKnob{Name: "innodb_file_per_table", Label: "File per table"},
		dbKnob{Name: "max_connections", Label: "Max connections"},
		dbKnob{Name: "table_open_cache", Label: "Table open cache"},
		dbKnob{Name: "table_definition_cache", Label: "Table definition cache"},
		dbKnob{Name: "thread_cache_size", Label: "Thread cache"},
		dbKnob{Name: "tmp_table_size", Label: "Tmp table size"},
		dbKnob{Name: "max_heap_table_size", Label: "Max HEAP table"},
		dbKnob{Name: "sort_buffer_size", Label: "Sort buffer"},
		dbKnob{Name: "read_buffer_size", Label: "Read buffer"},
		dbKnob{Name: "read_rnd_buffer_size", Label: "Random read buffer"},
		dbKnob{Name: "join_buffer_size", Label: "Join buffer"},
		dbKnob{Name: "key_buffer_size", Label: "MyISAM key buffer"},
		dbKnob{Name: "max_allowed_packet", Label: "Max allowed packet"},
		dbKnob{Name: "wait_timeout", Label: "Wait timeout"},
		dbKnob{Name: "interactive_timeout", Label: "Interactive timeout"},
		dbKnob{Name: "max_connect_errors", Label: "Max connect errors"},
		dbKnob{Name: "open_files_limit", Label: "Open files limit"},
		dbKnob{Name: "skip_name_resolve", Label: "Skip name resolve"},
	)
	if mariadb {
		out = append(out,
			dbKnob{Name: "query_cache_type", Label: "Query cache type"},
			dbKnob{Name: "query_cache_size", Label: "Query cache size"},
		)
	}
	return out
}

func (m *Manager) Config(ramGB int) *rpc.DBConfig {
	engine := m.Engine()
	st := &rpc.DBConfig{Engine: engine, ConfPath: mysqlConfPath(engine)}
	st.TotalRAM = memTotal()
	st.TotalRAMGB = int(st.TotalRAM / (1024 * 1024 * 1024))
	if st.TotalRAMGB < 1 {
		st.TotalRAMGB = 1
	}
	st.SuggestedGB = clampInt(st.TotalRAMGB, 1, 128)
	if ramGB <= 0 {
		if n := readSavedRAMGB(st.ConfPath); n > 0 {
			ramGB = n
		} else {
			ramGB = st.SuggestedGB
		}
	}
	st.RAMGB = clampInt(ramGB, 1, 128)
	if engine == "" {
		st.Message = "Install MySQL or MariaDB first"
		return st
	}
	st.Installed = true
	st.Active = mysqlServiceActive(engine)
	st.Version = mysqlVersionLine()
	mariadb := engine == "mariadb"
	rec := mysqlTune(st.RAMGB, mariadb)
	live := mysqlVariables()
	fileVals := readConfSettings(st.ConfPath)
	for _, k := range dbKnobs(mariadb) {
		item := rpc.DBConfigItem{Name: k.Name, Label: k.Label, Recommend: rec[k.Name]}
		if v := fileVals[k.Name]; v != "" {
			item.Value = v
		} else if k.Name == "bind_address" {
			if lv := live["bind_address"]; lv != "" {
				item.Value = lv
			} else {
				item.Value = "127.0.0.1"
			}
		} else if rec[k.Name] != "" {
			item.Value = rec[k.Name]
		}
		if lv := live[k.Name]; lv != "" {
			item.Live = prettyMySQLValue(k.Name, lv)
		}
		st.Settings = append(st.Settings, item)
	}
	return st
}

func (m *Manager) ApplyConfig(in rpc.DBConfigApply) error {
	engine := m.Engine()
	if engine == "" {
		return fmt.Errorf("MySQL/MariaDB is not installed")
	}
	ramGB := clampInt(in.RAMGB, 1, 128)
	mariadb := engine == "mariadb"
	vals := mysqlTune(ramGB, mariadb)
	allowed := map[string]bool{}
	for _, k := range dbKnobs(mariadb) {
		allowed[k.Name] = true
	}
	for name, v := range in.Settings {
		name = strings.TrimSpace(name)
		v = strings.TrimSpace(v)
		if !allowed[name] {
			return fmt.Errorf("unknown setting %s", name)
		}
		if v == "" || !validDBValue(name, v) {
			return fmt.Errorf("invalid value for %s", name)
		}
		vals[name] = v
	}
	if vals["bind_address"] == "" {
		vals["bind_address"] = "127.0.0.1"
	}
	path := mysqlConfPath(engine)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# managed by siroc\n# siroc-ram-gb=%d\n[mysqld]\n", ramGB)
	for _, k := range dbKnobs(mariadb) {
		v := vals[k.Name]
		if v == "" {
			continue
		}
		if k.Name == "skip_name_resolve" {
			if v == "1" || strings.EqualFold(v, "ON") {
				b.WriteString("skip_name_resolve = 1\n")
			}
			continue
		}
		if k.Name == "bind_address" {
			fmt.Fprintf(&b, "bind-address = %s\n", v)
			continue
		}
		fmt.Fprintf(&b, "%s = %s\n", k.Name, v)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		return err
	}
	if n, err := strconv.Atoi(vals["open_files_limit"]); err == nil && n >= 1024 {
		_ = writeMySQLLimits(engine, n)
	}
	return restartMySQL(engine)
}

func validDBValue(name, v string) bool {
	if name == "bind_address" {
		return validBindAddress(v)
	}
	return dbValueRe.MatchString(v)
}

func validBindAddress(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" || len(v) > 200 {
		return false
	}
	if v == "*" {
		return true
	}
	for _, p := range strings.Split(v, ",") {
		p = strings.TrimSpace(p)
		if p == "*" {
			continue
		}
		if net.ParseIP(p) == nil {
			return false
		}
	}
	return true
}

func mysqlTune(ramGB int, mariadb bool) map[string]string {
	ramGB = clampInt(ramGB, 1, 128)
	memMB := ramGB * 1024
	bufMB := memMB * 50 / 100
	switch ramGB {
	case 1:
		bufMB = 256
	case 2:
		bufMB = 512
	}
	if bufMB < 128 {
		bufMB = 128
	}
	instances := 1
	if bufMB >= 1024 {
		instances = bufMB / 1024
		if instances > 16 {
			instances = 16
		}
	}
	maxConn := clampInt(64+ramGB*24, 50, 4000)
	tableOpen := clampInt(256*ramGB, 256, 16384)
	tableDef := clampInt(tableOpen*2, 400, 20000)
	threadCache := clampInt(ramGB*8, 8, 256)
	tmpMB := clampInt(16+ramGB*8, 32, 512)
	keyMB := clampInt(8+ramGB*4, 8, 256)
	logBufMB := clampInt(8+ramGB, 8, 64)
	logFileMB := clampInt(bufMB/8, 48, 2048)
	redoMB := clampInt(bufMB/4, 64, 4096)
	sortKB := clampInt(256+ramGB*32, 256, 4096)
	readKB := clampInt(128+ramGB*16, 128, 2048)
	rndKB := clampInt(256+ramGB*32, 256, 4096)
	joinKB := clampInt(256+ramGB*16, 256, 2048)
	waitTO := 300
	if ramGB >= 8 {
		waitTO = 600
	}
	packet := "64M"
	if ramGB >= 8 {
		packet = "128M"
	}
	if ramGB >= 32 {
		packet = "256M"
	}
	openFiles := clampInt(tableOpen*2+maxConn+1024, 4096, 1048576)
	out := map[string]string{
		"innodb_buffer_pool_size":        sizeMB(bufMB),
		"innodb_buffer_pool_instances":   strconv.Itoa(instances),
		"innodb_log_buffer_size":         sizeMB(logBufMB),
		"innodb_flush_log_at_trx_commit": "1",
		"innodb_flush_method":            "O_DIRECT",
		"innodb_file_per_table":          "1",
		"max_connections":                strconv.Itoa(maxConn),
		"table_open_cache":               strconv.Itoa(tableOpen),
		"table_definition_cache":         strconv.Itoa(tableDef),
		"thread_cache_size":              strconv.Itoa(threadCache),
		"tmp_table_size":                 sizeMB(tmpMB),
		"max_heap_table_size":            sizeMB(tmpMB),
		"sort_buffer_size":               sizeKB(sortKB),
		"read_buffer_size":               sizeKB(readKB),
		"read_rnd_buffer_size":           sizeKB(rndKB),
		"join_buffer_size":               sizeKB(joinKB),
		"key_buffer_size":                sizeMB(keyMB),
		"max_allowed_packet":             packet,
		"wait_timeout":                   strconv.Itoa(waitTO),
		"interactive_timeout":            strconv.Itoa(waitTO),
		"max_connect_errors":             "100000",
		"skip_name_resolve":              "1",
		"open_files_limit":               strconv.Itoa(openFiles),
	}
	if mariadb {
		out["innodb_log_file_size"] = sizeMB(logFileMB)
		out["query_cache_type"] = "0"
		out["query_cache_size"] = "0"
	} else {
		out["innodb_redo_log_capacity"] = sizeMB(redoMB)
	}
	return out
}

func mysqlConfPath(engine string) string {
	if engine == "mariadb" {
		if st, err := os.Stat("/etc/mysql/mariadb.conf.d"); err == nil && st.IsDir() {
			return filepath.Join("/etc/mysql/mariadb.conf.d", sirocMySQLConfName)
		}
	}
	if st, err := os.Stat("/etc/mysql/mysql.conf.d"); err == nil && st.IsDir() {
		return filepath.Join("/etc/mysql/mysql.conf.d", sirocMySQLConfName)
	}
	_ = os.MkdirAll("/etc/mysql/conf.d", 0755)
	return filepath.Join("/etc/mysql/conf.d", sirocMySQLConfName)
}

func readSavedRAMGB(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# siroc-ram-gb=") {
			n, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "# siroc-ram-gb=")))
			return n
		}
	}
	return 0
}

func readConfSettings(path string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	section := ""
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.Trim(line, "[]"))
			continue
		}
		if section != "" && section != "mysqld" && section != "server" && section != "mariadbd" {
			continue
		}
		name, val, ok := strings.Cut(line, "=")
		if !ok {
			if line == "skip_name_resolve" || line == "skip-name-resolve" {
				out["skip_name_resolve"] = "1"
			}
			continue
		}
		name = strings.TrimSpace(strings.ReplaceAll(name, "-", "_"))
		val = strings.TrimSpace(val)
		out[name] = val
	}
	return out
}

func mysqlVariables() map[string]string {
	out := map[string]string{}
	raw, err := mysqlSelect("SHOW VARIABLES")
	if err != nil {
		return out
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		cols := strings.SplitN(line, "\t", 2)
		if len(cols) != 2 {
			continue
		}
		out[cols[0]] = cols[1]
	}
	return out
}

func mysqlSelect(sql string) (string, error) {
	bin := mysqlBin()
	flagSets := [][]string{
		{"--batch", "--raw", "--skip-column-names", "--skip-ssl"},
		{"--batch", "--raw", "--skip-column-names", "--ssl-mode=DISABLED"},
		{"--batch", "--raw", "--skip-column-names"},
	}
	var last error
	for _, flags := range flagSets {
		cmd := exec.Command(bin, append(flags, "-e", sql)...)
		b, err := cmd.CombinedOutput()
		if err == nil {
			return string(b), nil
		}
		last = fmt.Errorf("%s", strings.TrimSpace(string(b)))
	}
	if last == nil {
		last = fmt.Errorf("query failed")
	}
	return "", last
}

func mysqlVersionLine() string {
	out, _ := exec.Command(mysqlBin(), "--version").CombinedOutput()
	return strings.TrimSpace(string(out))
}

func mysqlServiceActive(engine string) bool {
	for _, name := range mysqlServiceNames(engine) {
		if exec.Command("systemctl", "is-active", "--quiet", name).Run() == nil {
			return true
		}
	}
	return false
}

func mysqlServiceNames(engine string) []string {
	if engine == "mariadb" {
		return []string{"mariadb", "mysql"}
	}
	return []string{"mysql", "mysqld", "mariadb"}
}

func restartMySQL(engine string) error {
	_ = exec.Command("systemctl", "daemon-reload").Run()
	var last error
	for _, name := range mysqlServiceNames(engine) {
		if exec.Command("systemctl", "cat", name).Run() != nil {
			continue
		}
		cmd := exec.Command("systemctl", "restart", name)
		out, err := cmd.CombinedOutput()
		if err == nil {
			deadline := time.Now().Add(45 * time.Second)
			for time.Now().Before(deadline) {
				if exec.Command("systemctl", "is-active", "--quiet", name).Run() == nil {
					return nil
				}
				time.Sleep(500 * time.Millisecond)
			}
			return fmt.Errorf("%s restarted but is not active yet", name)
		}
		last = fmt.Errorf("%s: %s", name, strings.TrimSpace(string(out)))
	}
	if last == nil {
		last = fmt.Errorf("could not restart %s", engine)
	}
	return last
}

func writeMySQLLimits(engine string, n int) error {
	unit := "mysql.service"
	if engine == "mariadb" {
		unit = "mariadb.service"
	}
	dir := filepath.Join("/etc/systemd/system", unit+".d")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	body := fmt.Sprintf("[Service]\nLimitNOFILE=%d\n", n)
	return os.WriteFile(filepath.Join(dir, "siroc-limits.conf"), []byte(body), 0644)
}

func memTotal() int64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
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

func prettyMySQLValue(name, raw string) string {
	switch name {
	case "innodb_buffer_pool_size", "innodb_log_buffer_size", "innodb_log_file_size",
		"innodb_redo_log_capacity", "tmp_table_size", "max_heap_table_size",
		"sort_buffer_size", "read_buffer_size", "read_rnd_buffer_size",
		"join_buffer_size", "key_buffer_size", "max_allowed_packet", "query_cache_size":
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n <= 0 {
			return raw
		}
		return humanBytes(n)
	case "skip_name_resolve", "innodb_file_per_table":
		if raw == "1" || strings.EqualFold(raw, "ON") {
			return "ON"
		}
		if raw == "0" || strings.EqualFold(raw, "OFF") {
			return "OFF"
		}
	}
	return raw
}

func humanBytes(n int64) string {
	if n%(1<<30) == 0 {
		return strconv.FormatInt(n>>30, 10) + "G"
	}
	if n%(1<<20) == 0 {
		return strconv.FormatInt(n>>20, 10) + "M"
	}
	if n%(1<<10) == 0 {
		return strconv.FormatInt(n>>10, 10) + "K"
	}
	if n >= 1<<30 {
		return fmt.Sprintf("%.1fG", float64(n)/(1<<30))
	}
	if n >= 1<<20 {
		return fmt.Sprintf("%.1fM", float64(n)/(1<<20))
	}
	return strconv.FormatInt(n, 10)
}

func sizeMB(mb int) string {
	if mb >= 1024 && mb%1024 == 0 {
		return strconv.Itoa(mb/1024) + "G"
	}
	return strconv.Itoa(mb) + "M"
}

func sizeKB(kb int) string {
	if kb >= 1024 && kb%1024 == 0 {
		return strconv.Itoa(kb/1024) + "M"
	}
	return strconv.Itoa(kb) + "K"
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

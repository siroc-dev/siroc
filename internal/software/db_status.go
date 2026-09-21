//go:build linux

package software

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func DBStatus(engine string) *rpc.DBStatus {
	st := &rpc.DBStatus{Engine: engine}
	ok, ver := dbInstalled(engine)
	st.Installed = ok
	st.Version = ver
	title := dbTitle(engine)
	if !ok {
		other := "mariadb"
		if engine == "mariadb" {
			other = "mysql"
		}
		if o, _ := dbInstalled(other); o {
			st.Message = dbTitle(other) + " is installed instead. Use the " + dbTitle(other) + " status page."
			return st
		}
		st.Message = title + " is not installed. Install it from Software."
		return st
	}
	svc := engine
	if engine == "mysql" && !serviceActive("mysql") && serviceActive("mysqld") {
		svc = "mysqld"
	}
	st.Active = serviceActive(svc)
	if !st.Active && engine == "mysql" {
		st.Active = serviceActive("mariadb") && mysqlLooksLike(engine)
	}
	if !st.Active {
		st.Message = title + " is installed but not running."
		return st
	}
	status, err := mysqlQuery(engine, "SHOW GLOBAL STATUS")
	if err != nil {
		st.Message = err.Error()
		return st
	}
	vars, _ := mysqlQuery(engine, "SHOW VARIABLES WHERE Variable_name IN ('version','version_comment','max_connections','datadir')")
	plist, _ := mysqlQuery(engine, "SHOW FULL PROCESSLIST")
	kv := parseMySQLKV(status)
	vk := parseMySQLKV(vars)
	st.UptimeSec = atoi64(kv["Uptime"])
	st.ThreadsConnected = atoi(kv["Threads_connected"])
	st.ThreadsRunning = atoi(kv["Threads_running"])
	st.MaxUsedConnections = atoi(kv["Max_used_connections"])
	st.MaxConnections = atoi(vk["max_connections"])
	st.Questions = atoi64(kv["Questions"])
	st.SlowQueries = atoi64(kv["Slow_queries"])
	st.Connections = atoi64(kv["Connections"])
	st.AbortedConnects = atoi64(kv["Aborted_connects"])
	st.BytesReceived = atoi64(kv["Bytes_received"])
	st.BytesSent = atoi64(kv["Bytes_sent"])
	st.OpenTables = atoi64(kv["Open_tables"])
	st.DataDir = vk["datadir"]
	if v := vk["version"]; v != "" {
		st.Version = v
	}
	st.Comment = vk["version_comment"]
	if st.UptimeSec > 0 {
		st.QPS = float64(st.Questions) / float64(st.UptimeSec)
		st.BytesPerSec = float64(st.BytesReceived+st.BytesSent) / float64(st.UptimeSec)
	}
	st.Processes = parseProcessList(plist)
	st.Ready = true
	st.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	return st
}

func dbInstalled(engine string) (bool, string) {
	if ok, ver := sqlSourceInstalled(engine); ok {
		return true, ver
	}
	if engine == "mysql" {
		if ok, ver := dpkgVersion("mysql-community-server"); ok {
			return true, ver
		}
		if ok, ver := dpkgVersion("mysql-server"); ok {
			return true, ver
		}
		return false, ""
	}
	return dpkgVersion("mariadb-server")
}

func mysqlLooksLike(engine string) bool {
	ok, _ := dbInstalled(engine)
	return ok
}

func dbTitle(engine string) string {
	if engine == "mariadb" {
		return "MariaDB"
	}
	return "MySQL"
}

func mysqlQuery(engine, sql string) (string, error) {
	bin := "mysql"
	if engine == "mariadb" {
		if _, err := exec.LookPath("mariadb"); err == nil {
			bin = "mariadb"
		}
	}
	flags := [][]string{
		{"--user=root", "--batch", "--raw", "--skip-column-names", "--skip-ssl"},
		{"--user=root", "--batch", "--raw", "--skip-column-names", "--ssl-mode=DISABLED"},
		{"--user=root", "--batch", "--raw", "--skip-column-names"},
		{"--batch", "--raw", "--skip-column-names", "--skip-ssl"},
	}
	var last string
	for _, base := range flags {
		cmd := exec.Command(bin, append(append([]string{}, base...), "-e", sql)...)
		out, err := combinedTimeout(cmd, 4*time.Second)
		if err == nil {
			return out, nil
		}
		last = tail(out)
	}
	return "", fmt.Errorf("%s: %s", bin, last)
}

func parseMySQLKV(raw string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, "\t")
		if !ok {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			k, v = fields[0], fields[len(fields)-1]
		}
		out[k] = v
	}
	return out
}

func parseProcessList(raw string) []rpc.DBProcess {
	var out []rpc.DBProcess
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) < 6 {
			continue
		}
		info := ""
		if len(cols) >= 8 {
			info = cols[7]
		}
		if len(info) > 240 {
			info = info[:240] + "…"
		}
		if strings.Contains(strings.ToUpper(info), "SHOW FULL PROCESSLIST") || strings.Contains(strings.ToUpper(info), "SHOW GLOBAL STATUS") {
			continue
		}
		state := ""
		if len(cols) >= 7 {
			state = nullDash(cols[6])
		}
		out = append(out, rpc.DBProcess{
			ID:      atoi64(cols[0]),
			User:    cols[1],
			Host:    cols[2],
			DB:      nullDash(cols[3]),
			Command: cols[4],
			Time:    atoi(cols[5]),
			State:   state,
			Info:    info,
		})
		if len(out) >= 40 {
			break
		}
	}
	return out
}

func nullDash(s string) string {
	if s == "NULL" || s == "null" {
		return ""
	}
	return s
}

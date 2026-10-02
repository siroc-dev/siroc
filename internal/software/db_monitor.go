//go:build linux

package software

import (
	"fmt"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func DatabasesMonitor(engine string) *rpc.DatabasesMonitor {
	return &rpc.DatabasesMonitor{
		SQL:   DBMonitor(engine),
		Redis: RedisStatus(),
	}
}

func DBMonitor(engine string) *rpc.DBSQLMonitor {
	if engine == "" {
		if ok, _ := dbInstalled("mariadb"); ok {
			engine = "mariadb"
		} else {
			engine = "mysql"
		}
	}
	st := &rpc.DBSQLMonitor{Engine: engine}
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
			st.Message = dbTitle(other) + " is installed instead."
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
	status, err := mysqlQuery(engine, "SHOW GLOBAL STATUS WHERE Variable_name IN ('Uptime','Questions','Slow_queries','Threads_connected','Threads_running')")
	if err != nil {
		st.Message = err.Error()
		return st
	}
	vars, _ := mysqlQuery(engine, "SHOW VARIABLES WHERE Variable_name IN ('version','slow_query_log','long_query_time','slow_query_log_file','log_queries_not_using_indexes','log_output')")
	kv := parseMySQLKV(status)
	vk := parseMySQLKV(vars)
	st.SlowQueryLog = mysqlOn(vk["slow_query_log"])
	st.LongQueryTime = vk["long_query_time"]
	st.SlowLogFile = vk["slow_query_log_file"]
	st.LogOutput = vk["log_output"]
	st.LogQueriesNotUsingIndexes = mysqlOn(vk["log_queries_not_using_indexes"])
	st.SlowQueries = parseInt64(kv["Slow_queries"])
	st.Questions = parseInt64(kv["Questions"])
	st.ThreadsConnected = int(parseInt64(kv["Threads_connected"]))
	st.ThreadsRunning = int(parseInt64(kv["Threads_running"]))
	if v := vk["version"]; v != "" {
		st.Version = v
	}
	if up := parseInt64(kv["Uptime"]); up > 0 {
		st.QPS = float64(st.Questions) / float64(up)
	}
	digest := `SELECT IFNULL(SCHEMA_NAME,''), COUNT_STAR, ROUND(AVG_TIMER_WAIT/1000000000, 2), ROUND(MAX_TIMER_WAIT/1000000000, 2), SUM_ROWS_EXAMINED, DATE_FORMAT(LAST_SEEN, '%Y-%m-%d %H:%i:%s'), IFNULL(DIGEST_TEXT,'') FROM performance_schema.events_statements_summary_by_digest WHERE DIGEST_TEXT IS NOT NULL AND UPPER(DIGEST_TEXT) NOT LIKE 'SHOW %' AND UPPER(DIGEST_TEXT) NOT LIKE 'SET %' ORDER BY SUM_TIMER_WAIT DESC LIMIT 25`
	if raw, err := mysqlQuery(engine, digest); err == nil {
		st.Recent = parseSlowDigest(raw)
	}
	if plist, err := mysqlQuery(engine, "SHOW FULL PROCESSLIST"); err == nil {
		st.Running = slowRunning(parseProcessList(plist), parseFloat64(st.LongQueryTime))
	}
	if !st.SlowQueryLog && st.SlowQueries == 0 && len(st.Recent) == 0 {
		st.Message = fmt.Sprintf("Slow query log is off. Enable slow_query_log in Config or my.cnf (long_query_time=%s).", orDash(st.LongQueryTime, "2"))
	}
	st.Ready = true
	st.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	return st
}

func orDash(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

package software

import (
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func mysqlOn(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "on", "1", "true", "yes":
		return true
	default:
		return false
	}
}

func parseSlowDigest(raw string) []rpc.SlowQuery {
	var out []rpc.SlowQuery
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) < 7 {
			continue
		}
		query := strings.Join(cols[6:], "\t")
		if len(query) > 400 {
			query = query[:400] + "…"
		}
		out = append(out, rpc.SlowQuery{
			Schema:       emptyNULL(cols[0]),
			Count:        parseInt64(cols[1]),
			AvgMs:        parseFloat64(cols[2]),
			MaxMs:        parseFloat64(cols[3]),
			RowsExamined: parseInt64(cols[4]),
			LastSeen:     emptyNULL(cols[5]),
			Query:        query,
		})
		if len(out) >= 25 {
			break
		}
	}
	return out
}

func slowRunning(list []rpc.DBProcess, longSec float64) []rpc.DBProcess {
	if longSec < 1 {
		longSec = 1
	}
	var out []rpc.DBProcess
	for _, p := range list {
		cmd := strings.ToLower(strings.TrimSpace(p.Command))
		if cmd == "sleep" || cmd == "binlog dump" || cmd == "connect" || cmd == "daemon" {
			continue
		}
		if float64(p.Time) < longSec {
			continue
		}
		if strings.TrimSpace(p.Info) == "" {
			continue
		}
		out = append(out, p)
		if len(out) >= 20 {
			break
		}
	}
	return out
}

func emptyNULL(s string) string {
	if s == "NULL" || s == "null" {
		return ""
	}
	return s
}

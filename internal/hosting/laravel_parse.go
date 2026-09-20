package hosting

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/validate"
)

func normalizeQueueSpecs(csv string, workers int, rows []rpc.LaravelQueueRow) ([]rpc.LaravelQueueRow, error) {
	if len(rows) > 0 {
		out := make([]rpc.LaravelQueueRow, 0, len(rows))
		seen := map[string]bool{}
		for _, row := range rows {
			name, err := validate.LaravelQueueNames(row.Name)
			if err != nil {
				return nil, err
			}
			if strings.Contains(name, ",") {
				return nil, fmt.Errorf("put each queue name on its own row")
			}
			n, err := validate.LaravelQueueWorkers(row.Workers)
			if err != nil {
				return nil, err
			}
			if seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, rpc.LaravelQueueRow{Name: name, Workers: n})
		}
		if len(out) == 0 {
			return []rpc.LaravelQueueRow{{Name: "default", Workers: 1}}, nil
		}
		if len(out) > 8 {
			return nil, fmt.Errorf("at most 8 queue names")
		}
		return out, nil
	}
	names, err := validate.LaravelQueueNames(csv)
	if err != nil {
		return nil, err
	}
	n, err := validate.LaravelQueueWorkers(workers)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(names, ",")
	out := make([]rpc.LaravelQueueRow, 0, len(parts))
	for _, part := range parts {
		out = append(out, rpc.LaravelQueueRow{Name: part, Workers: n})
	}
	return out, nil
}

func laravelQueueNameFromConf(conf string) string {
	for _, line := range strings.Split(conf, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "command=") {
			continue
		}
		idx := strings.Index(line, "--queue=")
		if idx < 0 {
			return "default"
		}
		rest := strings.TrimSpace(line[idx+len("--queue="):])
		if rest == "" {
			return "default"
		}
		name := strings.Fields(rest)[0]
		out, err := validate.LaravelQueueNames(name)
		if err != nil {
			return "default"
		}
		return out
	}
	return "default"
}

func laravelWorkersFromConf(conf string) int {
	for _, line := range strings.Split(conf, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "numprocs=") {
			n, _ := strconv.Atoi(strings.TrimPrefix(line, "numprocs="))
			if n > 0 {
				return n
			}
		}
	}
	return 1
}

func laravelLogFromConf(conf string) string {
	for _, line := range strings.Split(conf, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "stdout_logfile=") {
			return strings.TrimSpace(strings.TrimPrefix(line, "stdout_logfile="))
		}
	}
	return ""
}

type supervisorProc struct {
	Name   string
	Status string
	Detail string
	PID    int
	Uptime string
}

func parseSupervisorStatus(out string) []supervisorProc {
	var list []supervisorProc
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := fields[0]
		if i := strings.LastIndex(name, ":"); i >= 0 {
			name = name[i+1:]
		}
		st := supervisorProc{Name: name, Status: fields[1]}
		if len(fields) > 2 {
			st.Detail = strings.Join(fields[2:], " ")
		}
		st.PID, st.Uptime = parseSupervisorDetail(st.Detail)
		list = append(list, st)
	}
	return list
}

func parseSupervisorDetail(detail string) (pid int, uptime string) {
	for _, part := range strings.Split(detail, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "pid ") {
			pid, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(part, "pid ")))
		}
		if strings.HasPrefix(part, "uptime ") {
			uptime = strings.TrimSpace(strings.TrimPrefix(part, "uptime "))
		}
	}
	return pid, uptime
}

func lastRunFromUptime(now time.Time, uptime string) time.Time {
	d, ok := parseSupervisorUptime(uptime)
	if !ok {
		return time.Time{}
	}
	return now.Add(-d)
}

func parseSupervisorUptime(s string) (time.Duration, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	days := 0
	if i := strings.Index(s, " day"); i >= 0 {
		n, err := strconv.Atoi(strings.TrimSpace(s[:i]))
		if err != nil || n < 0 {
			return 0, false
		}
		days = n
		if j := strings.Index(s, ","); j >= 0 {
			s = strings.TrimSpace(s[j+1:])
		} else {
			return time.Duration(days) * 24 * time.Hour, true
		}
	}
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	sec, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, false
	}
	return time.Duration(days)*24*time.Hour + time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(sec)*time.Second, true
}

func queueNextRun(status string, last time.Time, now time.Time) string {
	switch strings.ToUpper(status) {
	case "RUNNING":
		return "listening"
	case "STARTING", "BACKOFF":
		return now.Add(5 * time.Second).UTC().Format(time.RFC3339)
	case "STOPPED", "EXITED", "FATAL", "UNKNOWN", "":
		if !last.IsZero() {
			return ""
		}
		return ""
	default:
		if !last.IsZero() {
			return last.Add(3 * time.Second).UTC().Format(time.RFC3339)
		}
	}
	return ""
}

func parseScheduleList(raw string) []rpc.LaravelSchedule {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if i := strings.Index(raw, "["); i >= 0 && strings.Contains(raw, "expression") {
		if events := parseScheduleJSON(raw[i:]); len(events) > 0 {
			return events
		}
	}
	return parseScheduleText(raw)
}

func parseScheduleJSON(raw string) []rpc.LaravelSchedule {
	var rows []struct {
		Expression   string `json:"expression"`
		Command      string `json:"command"`
		Description  string `json:"description"`
		NextDueDate  string `json:"next_due_date"`
		NextDueHuman string `json:"next_due_human"`
	}
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil
	}
	out := make([]rpc.LaravelSchedule, 0, len(rows))
	for _, r := range rows {
		cmd := strings.TrimSpace(r.Command)
		if cmd == "" {
			cmd = strings.TrimSpace(r.Description)
		}
		if cmd == "" {
			cmd = r.Expression
		}
		next := strings.TrimSpace(r.NextDueDate)
		if next == "" {
			next = strings.TrimSpace(r.NextDueHuman)
		}
		out = append(out, rpc.LaravelSchedule{Expression: r.Expression, Command: cmd, NextRun: next})
	}
	return out
}

func parseScheduleText(raw string) []rpc.LaravelSchedule {
	var out []rpc.LaravelSchedule
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Next Due") {
			continue
		}
		next := ""
		if i := strings.Index(line, "Next Due:"); i >= 0 {
			next = strings.TrimSpace(line[i+len("Next Due:"):])
			line = strings.TrimSpace(line[:i])
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		expr := strings.Join(fields[:5], " ")
		cmd := strings.Join(fields[5:], " ")
		cmd = strings.TrimRight(cmd, ". ")
		out = append(out, rpc.LaravelSchedule{Expression: expr, Command: cmd, NextRun: next})
	}
	return out
}

func formatRunTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

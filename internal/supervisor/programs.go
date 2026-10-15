package supervisor

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
)

const (
	confDir    = "/etc/supervisor/conf.d"
	defaultLog = "/var/log/supervisor"
)

var (
	nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
	userRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	pidRe  = regexp.MustCompile(`\bpid\s+(\d+)`)
)

var managedKeys = map[string]bool{
	"command": true, "directory": true, "user": true,
	"autostart": true, "autorestart": true, "numprocs": true,
	"stdout_logfile": true,
}

var knownStatus = map[string]bool{
	"RUNNING": true, "STARTING": true, "BACKOFF": true,
	"STOPPING": true, "STOPPED": true, "EXITED": true,
	"FATAL": true, "UNKNOWN": true,
}

type section struct {
	name   string
	lines  []string
	extra  []string
	prog   rpc.SupervisorProgram
	render bool
}

// Prepare checks a program the panel is allowed to write.
// Website programs (names starting with siroc-) stay under the site that created them.
func Prepare(req rpc.SupervisorProgramReq, existing *rpc.SupervisorProgram) (rpc.SupervisorProgram, error) {
	if existing != nil && existing.Site {
		return rpc.SupervisorProgram{}, fmt.Errorf("this program belongs to a website")
	}
	name := strings.TrimSpace(req.Name)
	if existing != nil {
		name = existing.Name
	}
	if !nameRe.MatchString(name) {
		return rpc.SupervisorProgram{}, fmt.Errorf("name must use letters, numbers, dot, underscore, or dash")
	}
	if existing == nil && strings.HasPrefix(name, "siroc-") {
		return rpc.SupervisorProgram{}, fmt.Errorf("names starting with siroc- belong to a website")
	}
	command := strings.TrimSpace(req.Command)
	if command == "" || strings.ContainsAny(command, "\r\n") || len(command) > 4000 {
		return rpc.SupervisorProgram{}, fmt.Errorf("command is required")
	}
	dir, err := cleanDir(req.Directory)
	if err != nil {
		return rpc.SupervisorProgram{}, err
	}
	user, err := cleanUser(req.User)
	if err != nil {
		return rpc.SupervisorProgram{}, err
	}
	workers := req.Numprocs
	if workers == 0 {
		workers = 1
	}
	if workers < 1 || workers > 32 {
		return rpc.SupervisorProgram{}, fmt.Errorf("workers must be from 1 to 32")
	}
	stdout := defaultLog + "/" + name + ".log"
	file := ""
	if existing != nil {
		if existing.StdoutLog != "" {
			stdout = existing.StdoutLog
		}
		file = existing.File
	}
	return rpc.SupervisorProgram{
		Name:        name,
		Command:     command,
		Directory:   dir,
		User:        user,
		Autostart:   req.Autostart,
		Autorestart: req.Autorestart,
		Numprocs:    workers,
		StdoutLog:   stdout,
		File:        file,
		Site:        strings.HasPrefix(name, "siroc-"),
	}, nil
}

func cleanDir(dir string) (string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "", nil
	}
	if strings.ContainsAny(dir, "\r\n\t ") {
		return "", fmt.Errorf("directory cannot contain spaces")
	}
	if !strings.HasPrefix(dir, "/") || strings.Contains(dir, "..") {
		return "", fmt.Errorf("directory must be an absolute path")
	}
	return pathClean(dir), nil
}

func cleanUser(user string) (string, error) {
	user = strings.TrimSpace(user)
	if user == "" {
		return "", nil
	}
	if !userRe.MatchString(user) {
		return "", fmt.Errorf("user must be a Linux account name")
	}
	return user, nil
}

func AllowedLog(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" || strings.Contains(p, "..") || strings.ContainsAny(p, "\r\n") || !strings.HasPrefix(p, "/") {
		return false
	}
	p = pathClean(p)
	for _, pre := range []string{"/var/log/", "/home/", "/tmp/"} {
		if strings.HasPrefix(p, pre) {
			return true
		}
	}
	return false
}

func pathClean(p string) string {
	parts := strings.Split(p, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		out = append(out, part)
	}
	return "/" + strings.Join(out, "/")
}

// ProgramsIn reads program blocks from one config file.
func ProgramsIn(file, text string) []rpc.SupervisorProgram {
	_, sections := splitConf(text)
	out := make([]rpc.SupervisorProgram, 0, len(sections))
	seen := map[string]bool{}
	for _, sec := range sections {
		if sec.name == "" || seen[sec.name] {
			continue
		}
		seen[sec.name] = true
		prog := sec.prog
		prog.File = file
		prog.Site = strings.HasPrefix(prog.Name, "siroc-")
		if prog.Numprocs < 1 {
			prog.Numprocs = 1
		}
		out = append(out, prog)
	}
	return out
}

// ApplyStatus attaches supervisorctl rows to programs.
// A process named program:program_00 stays under program; the full process name is kept.
func ApplyStatus(programs []rpc.SupervisorProgram, status string) []rpc.SupervisorProgram {
	out := append([]rpc.SupervisorProgram(nil), programs...)
	index := map[string]int{}
	for i := range out {
		index[out[i].Name] = i
		out[i].Processes = nil
	}
	for _, proc := range parseStatus(status) {
		key := groupKey(proc.Name)
		i, ok := index[key]
		if !ok {
			out = append(out, rpc.SupervisorProgram{
				Name:     key,
				Numprocs: 1,
				Site:     strings.HasPrefix(key, "siroc-"),
			})
			i = len(out) - 1
			index[key] = i
		}
		out[i].Processes = append(out[i].Processes, proc)
	}
	for i := range out {
		sort.Slice(out[i].Processes, func(a, b int) bool {
			return out[i].Processes[a].Name < out[i].Processes[b].Name
		})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	if out == nil {
		out = []rpc.SupervisorProgram{}
	}
	return out
}

func groupKey(process string) string {
	if i := strings.IndexByte(process, ':'); i > 0 {
		return process[:i]
	}
	return process
}

func parseStatus(text string) []rpc.SupervisorProc {
	var out []rpc.SupervisorProc
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !knownStatus[fields[1]] {
			continue
		}
		proc := rpc.SupervisorProc{Name: fields[0], Status: fields[1]}
		if len(fields) > 2 {
			proc.Detail = strings.Join(fields[2:], " ")
		}
		if m := pidRe.FindStringSubmatch(proc.Detail); len(m) == 2 {
			proc.PID, _ = strconv.Atoi(m[1])
		}
		if i := strings.Index(proc.Detail, "uptime "); i >= 0 {
			proc.Uptime = strings.TrimSpace(proc.Detail[i+len("uptime "):])
		}
		out = append(out, proc)
	}
	return out
}

// RewriteConf adds, replaces, or removes one program section.
// remove is true when the file no longer has anything to keep.
func RewriteConf(text, op string, prog rpc.SupervisorProgram) (string, bool, error) {
	head, sections := splitConf(text)
	switch op {
	case "add":
		for _, sec := range sections {
			if sec.name == prog.Name {
				return "", false, fmt.Errorf("program %s already exists", prog.Name)
			}
		}
		sections = append(sections, section{
			name:   prog.Name,
			prog:   prog,
			render: true,
			extra: []string{
				"redirect_stderr=true",
				"stdout_logfile_maxbytes=20MB",
				"stopwaitsecs=10",
			},
		})
	case "update":
		found := false
		for i := range sections {
			if sections[i].name != prog.Name {
				continue
			}
			if prog.StdoutLog == "" {
				prog.StdoutLog = sections[i].prog.StdoutLog
			}
			sections[i].prog = prog
			sections[i].render = true
			found = true
			break
		}
		if !found {
			return "", false, fmt.Errorf("program %s was not found", prog.Name)
		}
	case "delete":
		next := sections[:0]
		found := false
		for _, sec := range sections {
			if sec.name == prog.Name {
				found = true
				continue
			}
			next = append(next, sec)
		}
		if !found {
			return "", false, fmt.Errorf("program %s was not found", prog.Name)
		}
		sections = next
	default:
		return "", false, fmt.Errorf("unknown program action")
	}
	body := joinConf(head, sections)
	if strings.TrimSpace(body) == "" {
		return "", true, nil
	}
	return body, false, nil
}

func RenderProgram(prog rpc.SupervisorProgram, extra []string) string {
	workers := prog.Numprocs
	if workers < 1 {
		workers = 1
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[program:%s]\n", prog.Name)
	fmt.Fprintf(&b, "command=%s\n", prog.Command)
	if prog.Directory != "" {
		fmt.Fprintf(&b, "directory=%s\n", prog.Directory)
	}
	if prog.User != "" {
		fmt.Fprintf(&b, "user=%s\n", prog.User)
	}
	fmt.Fprintf(&b, "numprocs=%d\n", workers)
	fmt.Fprintf(&b, "autostart=%t\n", prog.Autostart)
	fmt.Fprintf(&b, "autorestart=%t\n", prog.Autorestart)
	if prog.StdoutLog != "" {
		fmt.Fprintf(&b, "stdout_logfile=%s\n", prog.StdoutLog)
	}
	for _, line := range extra {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func splitConf(text string) (string, []section) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	var head []string
	var sections []section
	var cur *section
	flush := func() {
		if cur == nil {
			return
		}
		fillSection(cur)
		sections = append(sections, *cur)
		cur = nil
	}
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "[") && strings.HasSuffix(trim, "]") && len(trim) > 2 {
			flush()
			cur = &section{lines: []string{line}}
			inner := strings.TrimSpace(trim[1 : len(trim)-1])
			if strings.HasPrefix(strings.ToLower(inner), "program:") {
				cur.name = strings.TrimSpace(inner[len("program:"):])
			}
			continue
		}
		if cur == nil {
			head = append(head, line)
			continue
		}
		cur.lines = append(cur.lines, line)
	}
	flush()
	return strings.Join(head, "\n"), sections
}

func fillSection(sec *section) {
	sec.prog.Name = sec.name
	sec.prog.Autostart = true
	sec.prog.Numprocs = 1
	for _, line := range sec.lines[1:] {
		key, val, ok := field(line)
		if !ok || !managedKeys[key] {
			if strings.TrimSpace(line) != "" {
				sec.extra = append(sec.extra, strings.TrimRight(line, "\r"))
			}
			continue
		}
		switch key {
		case "command":
			sec.prog.Command = val
		case "directory":
			sec.prog.Directory = val
		case "user":
			sec.prog.User = val
		case "autostart":
			sec.prog.Autostart = parseBool(val, true)
		case "autorestart":
			sec.prog.Autorestart = parseBool(val, false)
		case "numprocs":
			n, _ := strconv.Atoi(val)
			if n > 0 {
				sec.prog.Numprocs = n
			}
		case "stdout_logfile":
			sec.prog.StdoutLog = val
		}
	}
}

func field(line string) (string, string, bool) {
	trim := strings.TrimSpace(line)
	if trim == "" || strings.HasPrefix(trim, ";") || strings.HasPrefix(trim, "#") {
		return "", "", false
	}
	i := strings.IndexByte(trim, '=')
	if i <= 0 {
		return "", "", false
	}
	return strings.ToLower(strings.TrimSpace(trim[:i])), strings.TrimSpace(trim[i+1:]), true
}

func parseBool(v string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	default:
		return fallback
	}
}

func joinConf(head string, sections []section) string {
	var b strings.Builder
	if strings.TrimSpace(head) != "" {
		b.WriteString(strings.TrimRight(head, "\n"))
		b.WriteByte('\n')
	}
	for _, sec := range sections {
		block := strings.Join(sec.lines, "\n")
		if sec.render {
			block = RenderProgram(sec.prog, sec.extra)
		}
		b.WriteString(strings.TrimRight(block, "\n"))
		b.WriteByte('\n')
	}
	return b.String()
}

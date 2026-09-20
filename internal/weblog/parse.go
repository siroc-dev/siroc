package weblog

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
)

const (
	maxEntries      = 500
	maxMessageRunes = 2000
	maxContextRunes = 24000
)

var (
	laravelHeaderRe  = regexp.MustCompile(`^\[(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:[+-]\d\d:\d\d)?)\]\s*(?:([A-Za-z0-9_-]+)\.)?([A-Za-z]+):\s*(.*)$`)
	combinedAccessRe = regexp.MustCompile(`^(\S+) \S+ \S+ \[([^\]]+)\] "(\S+)\s+([^"]*?)\s*(HTTP/[^"]*)?" (\d{3})(?:\s+(\S+))?`)
	nginxErrorRe     = regexp.MustCompile(`^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}) \[(\w+)\]\s+(.*)$`)
	apacheErrorRe    = regexp.MustCompile(`^\[([A-Za-z]{3} [A-Za-z]{3}\s+\d{1,2} \d{2}:\d{2}:\d{2}(?:\.\d+)? \d{4})\] \[([^\]:]+):([^\]]+)\]\s+(.*)$`)
	wafBoundaryRe    = regexp.MustCompile(`(?m)^--([A-Za-z0-9]+)-A--\s*$`)
	wafPartRe        = regexp.MustCompile(`(?m)^--([A-Za-z0-9]+)-([A-Z])--\s*$`)
	wafHostRe        = regexp.MustCompile(`(?im)^Host:\s*(\S+)`)
	wafMsgRe         = regexp.MustCompile(`(?i)\[msg "([^"]+)"\]`)
	wafIDRe          = regexp.MustCompile(`(?i)\[id "(\d+)"\]`)
	wafSevRe         = regexp.MustCompile(`(?i)\[severity "([^"]+)"\]`)
	wafPartARe       = regexp.MustCompile(`^\[([^\]]+)\]`)
)

func ParseLogEntries(group, kind, content string) []rpc.SiteLogEntry {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	if strings.TrimSpace(content) == "" {
		return nil
	}
	var entries []rpc.SiteLogEntry
	switch {
	case group == "laravel" || kind == "app":
		entries = parseLaravel(content)
	case group == "waf" || kind == "audit":
		entries = parseWAF(content)
	case kind == "access":
		entries = parseAccess(content)
	case group == "nginx":
		entries = parseNginxError(content)
	default:
		entries = parseApacheError(content)
	}
	if len(entries) > maxEntries {
		entries = entries[len(entries)-maxEntries:]
	}
	reverseEntries(entries)
	return entries
}

func CountLevels(entries []rpc.SiteLogEntry) map[string]int {
	if len(entries) == 0 {
		return nil
	}
	out := make(map[string]int)
	for _, e := range entries {
		lvl := e.Level
		if lvl == "" {
			lvl = "info"
		}
		out[lvl]++
	}
	return out
}

func parseLaravel(content string) []rpc.SiteLogEntry {
	var entries []rpc.SiteLogEntry
	var cur *rpc.SiteLogEntry
	var ctx []string
	flush := func() {
		if cur == nil {
			return
		}
		cur.Context = clipRunes(strings.TrimSpace(strings.Join(ctx, "\n")), maxContextRunes)
		entries = append(entries, *cur)
		cur = nil
		ctx = nil
	}
	for _, line := range strings.Split(content, "\n") {
		m := laravelHeaderRe.FindStringSubmatch(line)
		if m != nil {
			flush()
			msg, extra := splitLaravelMessage(strings.TrimSpace(m[4]))
			e := rpc.SiteLogEntry{
				Time:    m[1],
				Env:     m[2],
				Level:   normalizeLevel(m[3]),
				Message: clipRunes(msg, maxMessageRunes),
			}
			cur = &e
			if extra != "" {
				ctx = append(ctx, extra)
			}
			continue
		}
		if cur == nil {
			if strings.TrimSpace(line) == "" {
				continue
			}
			e := rpc.SiteLogEntry{Level: "info", Message: clipRunes(line, maxMessageRunes)}
			cur = &e
			continue
		}
		ctx = append(ctx, line)
	}
	flush()
	return entries
}

func parseAccess(content string) []rpc.SiteLogEntry {
	var entries []rpc.SiteLogEntry
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		m := combinedAccessRe.FindStringSubmatch(line)
		if m == nil {
			entries = append(entries, rpc.SiteLogEntry{Level: "access", Message: clipRunes(line, maxMessageRunes)})
			continue
		}
		status, _ := strconv.Atoi(m[6])
		path := strings.TrimSpace(m[4])
		proto := strings.TrimSpace(m[5])
		msg := strings.TrimSpace(m[3] + " " + path)
		if proto != "" {
			msg += " " + proto
		}
		entries = append(entries, rpc.SiteLogEntry{
			Time:    m[2],
			Level:   accessLevel(status),
			Message: clipRunes(m[1]+" · "+msg+" · "+m[6], maxMessageRunes),
			Status:  status,
		})
	}
	return entries
}

func parseNginxError(content string) []rpc.SiteLogEntry {
	var entries []rpc.SiteLogEntry
	var cur *rpc.SiteLogEntry
	var ctx []string
	flush := func() {
		if cur == nil {
			return
		}
		cur.Context = clipRunes(strings.TrimSpace(strings.Join(ctx, "\n")), maxContextRunes)
		entries = append(entries, *cur)
		cur = nil
		ctx = nil
	}
	for _, line := range strings.Split(content, "\n") {
		m := nginxErrorRe.FindStringSubmatch(line)
		if m != nil {
			flush()
			e := rpc.SiteLogEntry{Time: m[1], Level: normalizeLevel(m[2]), Message: clipRunes(m[3], maxMessageRunes)}
			cur = &e
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		if cur == nil {
			e := rpc.SiteLogEntry{Level: "error", Message: clipRunes(line, maxMessageRunes)}
			cur = &e
			continue
		}
		ctx = append(ctx, line)
	}
	flush()
	return entries
}

func parseApacheError(content string) []rpc.SiteLogEntry {
	var entries []rpc.SiteLogEntry
	var cur *rpc.SiteLogEntry
	var ctx []string
	flush := func() {
		if cur == nil {
			return
		}
		cur.Context = clipRunes(strings.TrimSpace(strings.Join(ctx, "\n")), maxContextRunes)
		entries = append(entries, *cur)
		cur = nil
		ctx = nil
	}
	for _, line := range strings.Split(content, "\n") {
		m := apacheErrorRe.FindStringSubmatch(line)
		if m != nil {
			flush()
			e := rpc.SiteLogEntry{Time: m[1], Env: m[2], Level: normalizeLevel(m[3]), Message: clipRunes(m[4], maxMessageRunes)}
			cur = &e
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		if cur == nil {
			e := rpc.SiteLogEntry{Level: "error", Message: clipRunes(line, maxMessageRunes)}
			cur = &e
			continue
		}
		ctx = append(ctx, line)
	}
	flush()
	return entries
}

func parseWAF(content string) []rpc.SiteLogEntry {
	var entries []rpc.SiteLogEntry
	for _, rec := range splitWAFRecords(content) {
		if e, ok := parseWAFRecord(rec); ok {
			entries = append(entries, e)
		}
	}
	return entries
}

func FilterWAFByHost(content, host string) string {
	host = strings.TrimSpace(host)
	if host == "" || strings.TrimSpace(content) == "" {
		return ""
	}
	var b strings.Builder
	for _, rec := range splitWAFRecords(content) {
		if hostMatchesSite(wafRecordHost(rec), host) {
			b.WriteString(rec)
			if !strings.HasSuffix(rec, "\n") {
				b.WriteByte('\n')
			}
		}
	}
	return b.String()
}

func splitWAFRecords(content string) []string {
	idxs := wafBoundaryRe.FindAllStringIndex(content, -1)
	if len(idxs) == 0 {
		return nil
	}
	var out []string
	for i, idx := range idxs {
		end := len(content)
		if i+1 < len(idxs) {
			end = idxs[i+1][0]
		}
		out = append(out, content[idx[0]:end])
	}
	return out
}

func wafParts(rec string) map[string]string {
	matches := wafPartRe.FindAllStringSubmatchIndex(rec, -1)
	out := map[string]string{}
	for i, m := range matches {
		if len(m) < 6 {
			continue
		}
		name := rec[m[4]:m[5]]
		start := m[1]
		end := len(rec)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		out[name] = strings.TrimSpace(rec[start:end])
	}
	return out
}

func wafRecordHost(rec string) string {
	if m := wafHostRe.FindStringSubmatch(rec); m != nil {
		return m[1]
	}
	return ""
}

func hostMatchesSite(got, want string) bool {
	got = strings.ToLower(strings.TrimSpace(got))
	if i := strings.IndexByte(got, ':'); i >= 0 {
		got = got[:i]
	}
	got = strings.TrimSuffix(got, ".")
	want = strings.ToLower(strings.TrimSpace(want))
	return got == want || got == "www."+want
}

func parseWAFRecord(rec string) (rpc.SiteLogEntry, bool) {
	parts := wafParts(rec)
	a := parts["A"]
	b := parts["B"]
	h := parts["H"]
	if strings.TrimSpace(a+b+h) == "" {
		return rpc.SiteLogEntry{}, false
	}
	e := rpc.SiteLogEntry{Level: "warning"}
	if m := wafPartARe.FindStringSubmatch(strings.TrimSpace(a)); m != nil {
		e.Time = m[1]
	}
	reqLine := firstNonEmptyLine(b)
	if hm := wafHostRe.FindStringSubmatch(b); hm != nil && reqLine != "" {
		reqLine = reqLine + " · " + hm[1]
	}
	msg := ""
	if m := wafMsgRe.FindStringSubmatch(h); m != nil {
		msg = m[1]
	}
	if msg == "" {
		for _, line := range strings.Split(h, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(strings.ToLower(line), "message:") {
				msg = strings.TrimSpace(line[8:])
				break
			}
		}
	}
	if msg == "" {
		msg = reqLine
	} else if reqLine != "" {
		msg = msg + " · " + reqLine
	}
	if msg == "" {
		msg = "ModSecurity audit event"
	}
	e.Message = clipRunes(msg, maxMessageRunes)
	if m := wafIDRe.FindStringSubmatch(h); m != nil {
		e.Env = "id " + m[1]
	}
	if m := wafSevRe.FindStringSubmatch(h); m != nil {
		e.Level = normalizeLevel(m[1])
	}
	low := strings.ToLower(h + "\n" + rec)
	if strings.Contains(low, "access denied") {
		e.Level = "error"
	}
	ctx := strings.TrimSpace(h)
	if ctx == "" {
		ctx = strings.TrimSpace(b)
	}
	e.Context = clipRunes(ctx, maxContextRunes)
	return e, true
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func splitLaravelMessage(s string) (string, string) {
	for _, sep := range []string{` {"exception"`, ` {"`, " [{"} {
		if i := strings.Index(s, sep); i > 0 {
			return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i:])
		}
	}
	return s, ""
}

func normalizeLevel(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "emerg", "emergency":
		return "emergency"
	case "alert":
		return "alert"
	case "crit", "critical":
		return "critical"
	case "err", "error", "fatal":
		return "error"
	case "warn", "warning":
		return "warning"
	case "notice":
		return "notice"
	case "info", "information", "processing", "processed":
		return "info"
	case "debug", "trace":
		return "debug"
	case "failed":
		return "error"
	default:
		if s == "" {
			return "info"
		}
		return s
	}
}

func accessLevel(status int) string {
	switch {
	case status >= 500:
		return "error"
	case status >= 400:
		return "warning"
	default:
		return "info"
	}
}

func clipRunes(s string, n int) string {
	if n <= 0 || s == "" {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func reverseEntries(in []rpc.SiteLogEntry) {
	for i, j := 0, len(in)-1; i < j; i, j = i+1, j-1 {
		in[i], in[j] = in[j], in[i]
	}
}

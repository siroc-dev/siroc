package sysops

import (
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

// The scanner block is owned by security.SyncLocalVhostHosts and is not listed here.
var scannerBlockRe = regexp.MustCompile(`(?ms)\n?# BEGIN (?:SIROC|CP-SERVER)\n.*?# END (?:SIROC|CP-SERVER)\n?`)

var createdRe = regexp.MustCompile(`^created\s+(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z)\b\s*(.*)$`)

const maxHostEntries = 1000

type hostLine struct {
	raw     string
	ip      string
	names   []string
	created string
	note    string
	entry   bool
	id      int
}

func ListHostsText(text string) []rpc.HostEntry {
	body, _ := peelScanner(text)
	return entriesOf(parseHostLines(body))
}

func MutateHosts(text, op string, id int, ip string, names []string, now time.Time) (string, []rpc.HostEntry, error) {
	body, block := peelScanner(text)
	lines := parseHostLines(body)
	hadLocal := hasLoopbackLocalhost(lines)
	switch op {
	case "add":
		cleanIP, cleanNames, err := cleanHost(ip, names)
		if err != nil {
			return "", nil, err
		}
		if hostCount(lines) >= maxHostEntries {
			return "", nil, fmt.Errorf("too many host entries")
		}
		if nameTaken(lines, 0, cleanIP, cleanNames) {
			return "", nil, fmt.Errorf("that hostname is already on this address")
		}
		lines = append(lines, hostLine{
			entry:   true,
			ip:      cleanIP,
			names:   cleanNames,
			created: now.UTC().Format(time.RFC3339),
		})
	case "update":
		cleanIP, cleanNames, err := cleanHost(ip, names)
		if err != nil {
			return "", nil, err
		}
		ln := findHost(lines, id)
		if ln == nil {
			return "", nil, fmt.Errorf("host entry not found")
		}
		if nameTaken(lines, id, cleanIP, cleanNames) {
			return "", nil, fmt.Errorf("that hostname is already on this address")
		}
		ln.ip = cleanIP
		ln.names = cleanNames
	case "delete":
		if findHost(lines, id) == nil {
			return "", nil, fmt.Errorf("host entry not found")
		}
		next := lines[:0]
		for _, ln := range lines {
			if ln.entry && ln.id == id {
				continue
			}
			next = append(next, ln)
		}
		lines = next
	default:
		return "", nil, fmt.Errorf("unknown host action")
	}
	if hadLocal && !hasLoopbackLocalhost(lines) {
		return "", nil, fmt.Errorf("keep 127.0.0.1 localhost")
	}
	rendered := renderHostLines(lines)
	return joinScanner(rendered, block), entriesOf(parseHostLines(rendered)), nil
}

func peelScanner(text string) (body, block string) {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	loc := scannerBlockRe.FindStringIndex(text)
	if loc == nil {
		return text, ""
	}
	return text[:loc[0]] + text[loc[1]:], text[loc[0]:loc[1]]
}

func joinScanner(body, block string) string {
	body = strings.TrimRight(body, "\n")
	block = strings.Trim(block, "\n")
	if body == "" && block == "" {
		return ""
	}
	if block == "" {
		return body + "\n"
	}
	if body == "" {
		return block + "\n"
	}
	return body + "\n" + block + "\n"
}

func parseHostLines(text string) []hostLine {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	var out []hostLine
	nextID := 1
	for _, raw := range strings.Split(text, "\n") {
		ip, names, created, note, ok := parseHostLine(raw)
		if !ok {
			out = append(out, hostLine{raw: raw})
			continue
		}
		out = append(out, hostLine{entry: true, id: nextID, ip: ip, names: names, created: created, note: note})
		nextID++
	}
	return out
}

func parseHostLine(line string) (ip string, names []string, created, note string, ok bool) {
	raw := line
	comment := ""
	if i := strings.Index(raw, "#"); i >= 0 {
		comment = strings.TrimSpace(raw[i+1:])
		raw = raw[:i]
	}
	fields := strings.Fields(raw)
	if len(fields) < 2 || net.ParseIP(fields[0]) == nil {
		return "", nil, "", "", false
	}
	created, note = splitCreated(comment)
	return canonicalIP(fields[0]), fields[1:], created, note, true
}

func splitCreated(comment string) (created, note string) {
	m := createdRe.FindStringSubmatch(strings.TrimSpace(comment))
	if m == nil {
		return "", strings.TrimSpace(comment)
	}
	return m[1], strings.TrimSpace(m[2])
}

func renderHostLines(lines []hostLine) string {
	var b strings.Builder
	for i, ln := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		if !ln.entry {
			b.WriteString(ln.raw)
			continue
		}
		b.WriteString(ln.ip)
		b.WriteByte('\t')
		b.WriteString(strings.Join(ln.names, " "))
		comment := ""
		if ln.created != "" {
			comment = "created " + ln.created
		}
		if ln.note != "" {
			if comment != "" {
				comment += " "
			}
			comment += ln.note
		}
		if comment != "" {
			b.WriteString(" # ")
			b.WriteString(comment)
		}
	}
	if len(lines) > 0 {
		b.WriteByte('\n')
	}
	return b.String()
}

func entriesOf(lines []hostLine) []rpc.HostEntry {
	var out []rpc.HostEntry
	for _, ln := range lines {
		if !ln.entry {
			continue
		}
		names := append([]string(nil), ln.names...)
		out = append(out, rpc.HostEntry{ID: ln.id, IP: ln.ip, Names: names, Created: ln.created})
	}
	if out == nil {
		out = []rpc.HostEntry{}
	}
	return out
}

func findHost(lines []hostLine, id int) *hostLine {
	for i := range lines {
		if lines[i].entry && lines[i].id == id {
			return &lines[i]
		}
	}
	return nil
}

func hostCount(lines []hostLine) int {
	n := 0
	for _, ln := range lines {
		if ln.entry {
			n++
		}
	}
	return n
}

func hasLoopbackLocalhost(lines []hostLine) bool {
	for _, ln := range lines {
		if !ln.entry || ln.ip != "127.0.0.1" {
			continue
		}
		for _, n := range ln.names {
			if strings.EqualFold(n, "localhost") {
				return true
			}
		}
	}
	return false
}

func nameTaken(lines []hostLine, skipID int, ip string, names []string) bool {
	want := map[string]struct{}{}
	for _, n := range names {
		want[strings.ToLower(n)] = struct{}{}
	}
	for _, ln := range lines {
		if !ln.entry || ln.id == skipID || ln.ip != ip {
			continue
		}
		for _, n := range ln.names {
			if _, ok := want[strings.ToLower(n)]; ok {
				return true
			}
		}
	}
	return false
}

func cleanHost(ip string, names []string) (string, []string, error) {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return "", nil, fmt.Errorf("invalid IP address")
	}
	var out []string
	seen := map[string]struct{}{}
	for _, raw := range names {
		for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
		}) {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if !hostLabelOK(part) {
				return "", nil, fmt.Errorf("invalid hostname %s", part)
			}
			key := strings.ToLower(part)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return "", nil, fmt.Errorf("enter at least one hostname")
	}
	if len(out) > 32 {
		return "", nil, fmt.Errorf("too many hostnames")
	}
	return canonicalIP(parsed.String()), out, nil
}

func canonicalIP(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ip
	}
	if v4 := parsed.To4(); v4 != nil {
		return v4.String()
	}
	return parsed.String()
}

func hostLabelOK(n string) bool {
	if n == "" || len(n) > 253 {
		return false
	}
	for _, r := range n {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

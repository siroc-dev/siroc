package weblog

import (
	"regexp"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
)

var (
	journalISORe = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:[.,]\d+)?(?:Z|[+-]\d{2}:?\d{2})?)\s+\S+\s+(\S+?)(?:\[\d+\])?:\s*(.*)$`)
	syslogRe     = regexp.MustCompile(`^([A-Z][a-z]{2}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2})\s+\S+\s+(\S+?)(?:\[\d+\])?:\s*(.*)$`)
	stampSpaceRe = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}(?:[.,]\d+)?(?:Z|[+-]\d{2}:?\d{2})?)\s+(.*)$`)
	fail2banRe   = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2},\d+)\s+(\S+)\s+\[[^\]]+\]:\s+(\w+)\s+(.*)$`)
	mysqlRe      = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z?)\s+\d+\s+\[(\w+)\]\s+(.*)$`)
	redisRe      = regexp.MustCompile(`^\d+:\S+\s+(\d{1,2} [A-Z][a-z]{2} \d{4}(?: \d{2}:\d{2}:\d{2}(?:\.\d+)?)?)\s+([*#.\-])\s+(.*)$`)
	phpFpmRe     = regexp.MustCompile(`^\[(\d{2}-[A-Z][a-z]{2}-\d{4} \d{2}:\d{2}:\d{2})\]\s+(\w+):\s+(.*)$`)
)

func ParseSystemLogEntries(id, kind, content string) []rpc.SiteLogEntry {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	if strings.TrimSpace(content) == "" {
		return nil
	}
	id = strings.TrimSpace(id)
	kind = strings.TrimSpace(kind)
	var entries []rpc.SiteLogEntry
	switch {
	case strings.HasSuffix(id, "-access") || strings.Contains(id, "access"):
		entries = parseAccess(content)
	case id == "waf-audit" || kind == "audit":
		entries = parseWAF(content)
	case id == "nginx-error":
		entries = parseNginxError(content)
	case id == "apache-error":
		entries = parseApacheError(content)
	case id == "ssh-last" || kind == "command":
		entries = parseLast(content)
		return capEntries(entries, false)
	case strings.HasPrefix(id, "php-fpm-"):
		entries = parsePHPFpm(content)
	default:
		entries = parseSyslogish(content)
	}
	return capEntries(entries, true)
}

func capEntries(entries []rpc.SiteLogEntry, newestFirst bool) []rpc.SiteLogEntry {
	if len(entries) > maxEntries {
		if newestFirst {
			entries = entries[len(entries)-maxEntries:]
		} else {
			entries = entries[:maxEntries]
		}
	}
	if newestFirst {
		reverseEntries(entries)
	}
	return entries
}

func parseSyslogish(content string) []rpc.SiteLogEntry {
	var entries []rpc.SiteLogEntry
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "-- ") {
			continue
		}
		if e, ok := parseSystemLine(line); ok {
			entries = append(entries, e)
		}
	}
	return entries
}

func parseSystemLine(line string) (rpc.SiteLogEntry, bool) {
	if m := fail2banRe.FindStringSubmatch(line); m != nil {
		return rpc.SiteLogEntry{
			Time:    m[1],
			Env:     m[2],
			Level:   normalizeLevel(m[3]),
			Message: clipRunes(m[4], maxMessageRunes),
		}, true
	}
	if m := mysqlRe.FindStringSubmatch(line); m != nil {
		return rpc.SiteLogEntry{
			Time:    m[1],
			Level:   normalizeLevel(m[2]),
			Message: clipRunes(m[3], maxMessageRunes),
		}, true
	}
	if m := journalISORe.FindStringSubmatch(line); m != nil {
		return rpc.SiteLogEntry{
			Time:    m[1],
			Env:     m[2],
			Level:   inferLevel(m[3] + " " + m[2]),
			Message: clipRunes(m[3], maxMessageRunes),
		}, true
	}
	if m := syslogRe.FindStringSubmatch(line); m != nil {
		return rpc.SiteLogEntry{
			Time:    m[1],
			Env:     m[2],
			Level:   inferLevel(m[3] + " " + m[2]),
			Message: clipRunes(m[3], maxMessageRunes),
		}, true
	}
	if m := phpFpmRe.FindStringSubmatch(line); m != nil {
		return rpc.SiteLogEntry{
			Time:    m[1],
			Level:   normalizeLevel(m[2]),
			Message: clipRunes(m[3], maxMessageRunes),
		}, true
	}
	if m := redisRe.FindStringSubmatch(line); m != nil {
		return rpc.SiteLogEntry{
			Time:    m[1],
			Level:   redisMarkLevel(m[2]),
			Message: clipRunes(m[3], maxMessageRunes),
		}, true
	}
	if m := stampSpaceRe.FindStringSubmatch(line); m != nil {
		return rpc.SiteLogEntry{
			Time:    m[1],
			Level:   inferLevel(m[2]),
			Message: clipRunes(m[2], maxMessageRunes),
		}, true
	}
	return rpc.SiteLogEntry{Level: inferLevel(line), Message: clipRunes(line, maxMessageRunes)}, true
}

func parsePHPFpm(content string) []rpc.SiteLogEntry {
	var entries []rpc.SiteLogEntry
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if m := phpFpmRe.FindStringSubmatch(line); m != nil {
			entries = append(entries, rpc.SiteLogEntry{
				Time:    m[1],
				Level:   normalizeLevel(m[2]),
				Message: clipRunes(m[3], maxMessageRunes),
			})
			continue
		}
		if e, ok := parseSystemLine(line); ok {
			entries = append(entries, e)
		}
	}
	return entries
}

func parseLast(content string) []rpc.SiteLogEntry {
	var entries []rpc.SiteLogEntry
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "wtmp begins") {
			continue
		}
		entries = append(entries, rpc.SiteLogEntry{
			Level:   inferLevel(line),
			Message: clipRunes(line, maxMessageRunes),
		})
	}
	return entries
}

func redisMarkLevel(mark string) string {
	switch mark {
	case "#":
		return "warning"
	case "-":
		return "error"
	case ".":
		return "debug"
	default:
		return "info"
	}
}

func inferLevel(s string) string {
	low := strings.ToLower(s)
	switch {
	case strings.Contains(low, "emerg"):
		return "emergency"
	case strings.Contains(low, "alert"):
		return "alert"
	case strings.Contains(low, "crit"):
		return "critical"
	case strings.Contains(low, "error"), strings.Contains(low, "fail"), strings.Contains(low, "denied"), strings.Contains(low, "fatal"):
		return "error"
	case strings.Contains(low, "warn"):
		return "warning"
	case strings.Contains(low, "notice"):
		return "notice"
	case strings.Contains(low, "debug"), strings.Contains(low, "trace"):
		return "debug"
	default:
		return "info"
	}
}

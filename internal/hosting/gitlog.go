package hosting

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/siroc-dev/siroc/internal/rpc"
)

// ParseGitLog reads `git log -1 --format=%H%n%s%n%an%n%aI`, including the command echo in front.
func ParseGitLog(out string) (rpc.GitCommit, bool) {
	lines := strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")
	for i, line := range lines {
		hash := strings.TrimSpace(line)
		if !gitHash(hash) || i+2 >= len(lines) {
			continue
		}
		subject := cleanGitText(lines[i+1], 200)
		if subject == "" {
			continue
		}
		c := rpc.GitCommit{
			Hash:    hash[:12],
			Subject: subject,
			Author:  cleanGitText(lines[i+2], 80),
		}
		if i+3 < len(lines) {
			at := strings.TrimSpace(lines[i+3])
			if _, err := time.Parse(time.RFC3339, at); err == nil {
				c.At = at
			}
		}
		return c, true
	}
	return rpc.GitCommit{}, false
}

func gitHash(s string) bool {
	if len(s) < 7 || len(s) > 40 {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func cleanGitText(s string, max int) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' {
			b.WriteByte(' ')
			continue
		}
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
	}
	out := strings.Join(strings.Fields(b.String()), " ")
	if utf8.RuneCountInString(out) <= max {
		return out
	}
	runes := []rune(out)
	return string(runes[:max])
}

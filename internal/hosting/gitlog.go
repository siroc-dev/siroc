package hosting

import (
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/validate"
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

// ParseSymrefHead reads the branch from `git ls-remote --symref origin HEAD`.
func ParseSymrefHead(out string) (string, bool) {
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "ref:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.HasPrefix(fields[1], "refs/heads/") {
			continue
		}
		name := strings.TrimPrefix(fields[1], "refs/heads/")
		if name == "" || strings.Contains(name, "..") {
			continue
		}
		return name, true
	}
	return "", false
}

// ParseRemoteBranches reads branch names from `git ls-remote --symref`.
// The default branch, when the output names one, is first.
func ParseRemoteBranches(out string) (branches []string, head string) {
	head, _ = ParseSymrefHead(out)
	if _, err := validate.GitBranch(head); head != "" && err != nil {
		head = ""
	}
	seen := map[string]struct{}{}
	var list []string
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "$") || strings.HasPrefix(line, "ref:") {
			continue
		}
		fields := strings.Fields(line)
		ref := fields[len(fields)-1]
		if !strings.HasPrefix(ref, "refs/heads/") {
			continue
		}
		name := strings.TrimPrefix(ref, "refs/heads/")
		if name == "" {
			continue
		}
		if _, err := validate.GitBranch(name); err != nil {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		list = append(list, name)
	}
	sort.Strings(list)
	if head != "" {
		ordered := []string{head}
		for _, name := range list {
			if name != head {
				ordered = append(ordered, name)
			}
		}
		list = ordered
	}
	if len(list) > 500 {
		list = list[:500]
	}
	return list, head
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

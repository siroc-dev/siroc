package security

import (
	"regexp"
	"strconv"
	"strings"
)

var ufwListRule = regexp.MustCompile(`^\[\s*(\d+)\].*#\s*(siroc-whitelist|siroc-blacklist)\b`)

// FirewallCommentIDs returns numbered ufw rules tagged as the Siroc whitelist or blacklist.
func FirewallCommentIDs(status string) []int {
	var ids []int
	for _, line := range strings.Split(status, "\n") {
		m := ufwListRule.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		id, _ := strconv.Atoi(m[1])
		if id > 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

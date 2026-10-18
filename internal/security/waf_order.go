package security

import (
	"sort"
	"strconv"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func ParseWAFPin(raw string) []int {
	var out []int
	for _, part := range strings.Split(raw, ",") {
		id, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || id < 100 {
			continue
		}
		out = append(out, id)
	}
	return out
}

func orderWAFRules(rules []rpc.WAFRule, pin []int, limit int) []rpc.WAFRule {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	prefer := map[int]bool{}
	for _, id := range pin {
		if id >= 100 {
			prefer[id] = true
		}
	}
	sort.SliceStable(rules, func(i, j int) bool {
		pi, pj := prefer[rules[i].ID], prefer[rules[j].ID]
		if pi != pj {
			return pi
		}
		return rules[i].ID < rules[j].ID
	})
	if len(rules) > limit {
		rules = append([]rpc.WAFRule(nil), rules[:limit]...)
	}
	if rules == nil {
		return []rpc.WAFRule{}
	}
	return rules
}

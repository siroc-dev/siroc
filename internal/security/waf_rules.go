//go:build linux

package security

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

var (
	wafIDRe   = regexp.MustCompile(`(?i)\bid:(\d{3,9})\b`)
	wafMsgRe  = regexp.MustCompile(`(?i)\bmsg:'([^']+)'`)
	ruleCache struct {
		mu    sync.Mutex
		at    time.Time
		rules []rpc.WAFRule
	}
)

func normalizeRuleIDs(ids []int) ([]int, error) {
	seen := map[int]bool{}
	var out []int
	for _, id := range ids {
		if id < 100 || id > 999999999 {
			return nil, fmt.Errorf("invalid rule id %d", id)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	sort.Ints(out)
	if out == nil {
		out = []int{}
	}
	return out, nil
}

func (m *Manager) WAFRules(q, pack string, limit int) (*rpc.WAFRulesResp, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	st := loadWAFState()
	disabled := map[int]bool{}
	for _, id := range st.DisabledIDs {
		disabled[id] = true
	}
	q = strings.ToLower(strings.TrimSpace(q))
	pack = strings.ToLower(strings.TrimSpace(pack))
	var matched []rpc.WAFRule
	for _, r := range catalogCRSRules() {
		r.Disabled = disabled[r.ID]
		if pack != "" && !strings.EqualFold(r.Pack, pack) {
			continue
		}
		if q != "" {
			id := strconv.Itoa(r.ID)
			if !strings.Contains(id, q) && !strings.Contains(strings.ToLower(r.Msg), q) && !strings.Contains(strings.ToLower(r.File), q) {
				continue
			}
		}
		matched = append(matched, r)
	}
	total := len(matched)
	if len(matched) > limit {
		matched = matched[:limit]
	}
	if matched == nil {
		matched = []rpc.WAFRule{}
	}
	return &rpc.WAFRulesResp{Rules: matched, Total: total}, nil
}

func catalogCRSRules() []rpc.WAFRule {
	ruleCache.mu.Lock()
	defer ruleCache.mu.Unlock()
	if time.Since(ruleCache.at) < 5*time.Minute && ruleCache.rules != nil {
		return ruleCache.rules
	}
	dir := crsRulesDir()
	if dir == "" {
		ruleCache.rules = []rpc.WAFRule{}
		ruleCache.at = time.Now()
		return ruleCache.rules
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.conf"))
	sort.Strings(files)
	seen := map[int]bool{}
	var out []rpc.WAFRule
	for _, f := range files {
		base := filepath.Base(f)
		pack := packForFile(base)
		for _, r := range parseRuleFile(f, base, pack) {
			if seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	ruleCache.rules = out
	ruleCache.at = time.Now()
	return out
}

func packForFile(name string) string {
	for _, p := range wafPackCatalog() {
		for _, prefix := range p.Prefixes {
			if strings.HasPrefix(name, prefix) {
				return p.ID
			}
		}
	}
	return ""
}

func parseRuleFile(path, file, pack string) []rpc.WAFRule {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var keep []string
	for _, line := range strings.Split(string(b), "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		keep = append(keep, line)
	}
	text := strings.Join(keep, "\n")
	text = strings.ReplaceAll(text, "\\\r\n", " ")
	text = strings.ReplaceAll(text, "\\\n", " ")
	parts := strings.Split(text, "SecRule")
	var out []rpc.WAFRule
	seen := map[int]bool{}
	for _, part := range parts[1:] {
		block := "SecRule" + part
		idm := wafIDRe.FindStringSubmatch(block)
		if idm == nil {
			continue
		}
		id, _ := strconv.Atoi(idm[1])
		if id < 100 || seen[id] {
			continue
		}
		if !strings.Contains(block, "msg:") && (strings.Contains(block, "skipAfter") || strings.Contains(block, "nolog")) {
			continue
		}
		msg := ""
		if mm := wafMsgRe.FindStringSubmatch(block); mm != nil {
			msg = mm[1]
		}
		if msg == "" {
			msg = file
		}
		seen[id] = true
		out = append(out, rpc.WAFRule{ID: id, Msg: msg, File: file, Pack: pack})
	}
	return out
}

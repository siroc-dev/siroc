package security

import "sort"

const wafDefaultsVersion = 2

func defaultOffWAFPacks() []string {
	return []string{"scanner", "session", "reputation", "dos"}
}

func defaultWAFPacks() []string {
	return []string{"protocol", "lfi", "rfi", "rce", "php", "generic", "xss", "sqli", "leakage"}
}

// defaultSEOWAFRules are CRS rules left off on Linux hosting panels.
// 913xxx: scanner/bot UA (SEO crawlers). Whole REQUEST-913 pack is also off.
// 920270-920274: non-ASCII in the request (Thai slugs and search).
// 920300-920311: missing or empty Accept (Facebook, Slack, and other link previews).
// 932110-932140 / 932370-932380: Windows CMD and PowerShell (this panel is Linux).
func defaultSEOWAFRules() []int {
	return []int{
		913100, 913101, 913102, 913110, 913120,
		920270, 920271, 920272, 920273, 920274,
		920300, 920310, 920311,
		932110, 932115, 932120, 932125, 932140,
		932370, 932371, 932380,
	}
}

func mergeWAFRuleIDs(cur, extra []int) []int {
	seen := map[int]bool{}
	var out []int
	for _, id := range append(append([]int{}, cur...), extra...) {
		if id < 100 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	sort.Ints(out)
	if out == nil {
		out = []int{}
	}
	return out
}

func dropWAFPack(packs []string, id string) []string {
	return dropWAFPacks(packs, id)
}

func dropWAFPacks(packs []string, ids ...string) []string {
	skip := map[string]bool{}
	for _, id := range ids {
		skip[id] = true
	}
	var out []string
	for _, p := range packs {
		if skip[p] {
			continue
		}
		out = append(out, p)
	}
	if out == nil {
		out = []string{}
	}
	return out
}

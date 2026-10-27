package security

import "sort"

const wafDefaultsVersion = 5

type wafState struct {
	Mode              string   `json:"mode"`
	Paranoia          int      `json:"paranoia"`
	InboundThreshold  int      `json:"inboundThreshold"`
	OutboundThreshold int      `json:"outboundThreshold"`
	Audit             string   `json:"audit"`
	Packs             []string `json:"packs"`
	DisabledIDs       []int    `json:"disabledIds"`
	UploadScan        bool     `json:"uploadScan"`
	SEORelaxed        int      `json:"seoRelaxed,omitempty"`
}

func defaultOffWAFPacks() []string {
	return []string{"scanner", "session", "reputation", "dos", "leakage"}
}

func defaultWAFPacks() []string {
	return []string{"protocol", "lfi", "rfi", "rce", "php", "generic", "xss", "sqli"}
}

// defaultSEOWAFRules are CRS rules left off on Linux hosting panels.
// 913xxx: scanner/bot UA (SEO crawlers). Whole REQUEST-913 pack is also off.
// 920270-920274: non-ASCII in the request (Thai slugs and search).
// 920300-920311: missing or empty Accept (Facebook, Slack, and other link previews).
// 932110-932140 / 932370-932380: Windows CMD and PowerShell (this panel is Linux).
// 941100 / 941160: XSS filters that false-positive on WordPress admin and HTML editors.
func defaultSEOWAFRules() []int {
	return []int{
		913100, 913101, 913102, 913110, 913120,
		920270, 920271, 920272, 920273, 920274,
		920300, 920310, 920311,
		932110, 932115, 932120, 932125, 932140,
		932370, 932371, 932380,
		941100, 941160,
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

func applyWAFStateDefaults(st wafState) wafState {
	if st.Mode == "" {
		st.Mode = "DetectionOnly"
	}
	if st.Paranoia < 1 || st.Paranoia > 4 {
		st.Paranoia = 1
	}
	if st.InboundThreshold < 1 {
		st.InboundThreshold = 5
	}
	if st.OutboundThreshold < 1 {
		st.OutboundThreshold = 4
	}
	switch st.Audit {
	case "On", "Off", "RelevantOnly":
	default:
		st.Audit = "RelevantOnly"
	}
	if st.Packs == nil {
		st.Packs = defaultWAFPacks()
	}
	if st.SEORelaxed < wafDefaultsVersion {
		st.DisabledIDs = mergeWAFRuleIDs(st.DisabledIDs, defaultSEOWAFRules())
		st.Packs = dropWAFPacks(st.Packs, defaultOffWAFPacks()...)
		st.SEORelaxed = wafDefaultsVersion
	}
	if st.DisabledIDs == nil {
		st.DisabledIDs = []int{}
	}
	return st
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

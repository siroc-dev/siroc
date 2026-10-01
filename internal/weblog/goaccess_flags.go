package weblog

import (
	"regexp"
	"strings"

	"github.com/siroc-dev/siroc/internal/version"
)

var goaccessVerRe = regexp.MustCompile(`(?i)goaccess\s*-?\s*([0-9]+(?:\.[0-9]+){1,3})`)

var goaccessExtraPanels = []string{"VISIT_TIMES", "GEO_LOCATION", "ASN", "AI_CRAWLERS"}

func goaccessReportArgs(help string, logs []string, out, days, title string, dbs []string, skip map[string]bool) []string {
	args := append([]string{}, logs...)
	args = append(args, "--no-global-config", "--log-format=COMBINED", "-a")
	if strings.Contains(help, "keep-last") && !skip["KEEP_LAST"] {
		args = append(args, "--keep-last", days)
	}
	if len(dbs) > 0 && strings.Contains(help, "geoip-database") {
		if strings.Contains(help, "enable-geoip") {
			args = append(args, "--enable-geoip=mmdb")
		}
		for _, db := range dbs {
			args = append(args, "--geoip-database="+db)
		}
	}
	if strings.Contains(help, "enable-panel") {
		for _, p := range goaccessExtraPanels {
			if skip[p] {
				continue
			}
			args = append(args, "--enable-panel="+p)
		}
	}
	args = append(args, "--html-report-title", title, "-o", out)
	return args
}

func parseGoaccessVersion(raw string) string {
	m := goaccessVerRe.FindStringSubmatch(raw)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func goaccessAtLeast(haveRaw, want string) bool {
	have := parseGoaccessVersion(haveRaw)
	if have == "" {
		return false
	}
	return version.Compare(have, want) >= 0
}

func goaccessSkipFromError(errOut string, skip map[string]bool) bool {
	added := false
	up := strings.ToUpper(errOut)
	for _, p := range goaccessExtraPanels {
		if skip[p] {
			continue
		}
		if strings.Contains(up, p) || strings.Contains(errOut, p) {
			skip[p] = true
			added = true
		}
	}
	if strings.Contains(errOut, "keep-last") {
		skip["KEEP_LAST"] = true
		added = true
	}
	return added
}

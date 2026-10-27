package security

import (
	"testing"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func TestSummarizeWPScanTable(t *testing.T) {
	body := `{
  "target_url": "http://doujin89.com/",
  "version": {
    "number": "6.4.2",
    "status": "outdated",
    "latest_version": "6.4.3",
    "vulnerabilities": [
      {"title": "WP < 6.4.3 XSS", "fixed_in": "6.4.3", "references": {"cve": ["2024-4439"]}}
    ]
  },
  "plugins": {
    "akismet": {"slug": "akismet", "number": "5.2", "outdated": true, "latest_version": "5.3", "vulnerabilities": []}
  },
  "users": {"admin": {"id": 1}},
  "interesting_findings": [{"to_s": "Headers", "interesting_entries": ["Server: nginx"]}]
}`
	st := &rpc.ScanResp{}
	summarizeWPScan(st, body)
	high, med, low, info := 0, 0, 0, 0
	for _, f := range st.Findings {
		switch f.Severity {
		case "high":
			high++
		case "medium":
			med++
		case "low":
			low++
		default:
			info++
		}
	}
	if high < 1 || med < 1 || low < 1 || info < 1 {
		t.Fatalf("want mixed findings, got high=%d medium=%d low=%d info=%d n=%d", high, med, low, info, len(st.Findings))
	}
	if st.Title != "WordPress check (WPScan)" {
		t.Fatalf("title %q", st.Title)
	}
}

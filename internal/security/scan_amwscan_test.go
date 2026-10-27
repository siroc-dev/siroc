package security

import (
	"testing"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func TestSummarizeAMWScanTable(t *testing.T) {
	body := `{
  "directory": "/home/alice/domains/site.com/public_html",
  "scanned": 40,
  "detected": 2,
  "files": {
    "/home/alice/domains/site.com/public_html/wp-content/uploads/x.php": {
      "status": "infected",
      "severity": "high",
      "exploits": ["eval", "base64_decode"]
    },
    "/home/alice/domains/site.com/public_html/theme.php": {
      "status": "warning",
      "severity": "keyword",
      "reason": "generic keyword match"
    }
  }
}`
	st := &rpc.ScanResp{}
	summarizeAMWScan(st, body)
	high, info := 0, 0
	for _, f := range st.Findings {
		switch f.Severity {
		case "high":
			high++
		case "info":
			info++
		}
	}
	if high < 1 || info < 1 {
		t.Fatalf("want infected + keyword, got high=%d info=%d n=%d", high, info, len(st.Findings))
	}
	if st.Title != "PHP antimalware scan" {
		t.Fatalf("title %q", st.Title)
	}
}

func TestSummarizeAMWScanCanonicalFindings(t *testing.T) {
	body := `{
  "path": "/home/alice/public_html",
  "findings": [
    {"kind":"malware","rule_id":"eval","subject":"/home/alice/public_html/shell.php","severity":"high","message":"eval obfuscation"},
    {"kind":"keyword","rule_id":"backdoor","subject":"/home/alice/public_html/theme.php","severity":"info","message":"generic keyword"}
  ]
}`
	st := &rpc.ScanResp{}
	summarizeAMWScan(st, body)
	if len(st.Findings) != 2 {
		t.Fatalf("findings %d", len(st.Findings))
	}
	if st.Findings[0].Severity != "high" {
		t.Fatalf("severity %q", st.Findings[0].Severity)
	}
}

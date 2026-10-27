package security

import (
	"strings"
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

func TestSummarizeAMWScanInfectedFound(t *testing.T) {
	body := `{
  "scanned": 12,
  "detected": 1,
  "infectedFound": {
    "/home/alice/public_html/shell.php": [
      {"type":"exploit","key":"eval","level":"danger","description":"eval obfuscation","line":4}
    ]
  }
}`
	st := &rpc.ScanResp{}
	summarizeAMWScan(st, body)
	if len(st.Findings) != 1 || st.Findings[0].Severity != "high" {
		t.Fatalf("findings %#v", st.Findings)
	}
	if !strings.Contains(st.Findings[0].Title, "shell.php") {
		t.Fatalf("title %q", st.Findings[0].Title)
	}
}

func TestSummarizeAMWScanTextReport(t *testing.T) {
	body := `PHP Antimalware Scanner Report
Version: 0.21.15
Total files scanned: 40
Infected files: 1

File: /home/alice/public_html/shell.php
 Size: 1 KB
 - Exploit eval [Line: 4, Dangerous]
 Description: eval obfuscation
 Match: eval($_POST["c"])
`
	st := &rpc.ScanResp{OK: true}
	summarizeAMWScan(st, body)
	if len(st.Findings) != 1 || st.Findings[0].Severity != "high" {
		t.Fatalf("findings %#v", st.Findings)
	}
	if !strings.Contains(st.Findings[0].Title, "shell.php") || !strings.Contains(st.Findings[0].Detail, "eval obfuscation") {
		t.Fatalf("finding %#v", st.Findings[0])
	}
}

func TestSummarizeAMWScanCleanTextReport(t *testing.T) {
	body := "PHP Antimalware Scanner Report\nTotal files scanned: 12\nInfected files: 0\n\nNo malware found!\n"
	st := &rpc.ScanResp{}
	summarizeAMWScan(st, body)
	if len(st.Findings) != 0 || !strings.Contains(st.Summary, "No malicious PHP") || !strings.Contains(st.Summary, "12") {
		t.Fatalf("summary %q findings %#v", st.Summary, st.Findings)
	}
}

func TestSummarizeAMWScanExitStatus(t *testing.T) {
	body := "\r[>                    ] 0% 23/7574 [2 sec/11 min]\r[>                    ] 0% 24/7574 [2 sec/10 min]\nPHP Fatal error: Allowed memory size of 134217728 bytes exhausted in /opt/siroc/amwscan/scanner on line 9\nexit status 255\n"
	collapsed := collapseScanOutput(body)
	if strings.Count(collapsed, "23/7574") != 0 || !strings.Contains(collapsed, "24/7574") || !strings.Contains(collapsed, "Allowed memory") {
		t.Fatalf("collapsed %q", collapsed)
	}
	st := &rpc.ScanResp{OK: true}
	summarizeAMWScan(st, collapsed)
	if st.OK || !strings.Contains(st.Summary, "Allowed memory") || strings.Contains(st.Summary, "exit status") {
		t.Fatalf("summary %q ok %v", st.Summary, st.OK)
	}
}

func TestSummarizeAMWScanIgnoresBanner(t *testing.T) {
	body := "  |A ||||W ||||S ||||\nCreated by Marco Cesarato\nPHP Antimalware Scanner\nversion 0.21.15\nGithub: https://github.com/marcocesarato/PHP-Antimalware-Scanner\ntimed out after 15m0s\n"
	st := &rpc.ScanResp{OK: true}
	summarizeAMWScan(st, body)
	if st.OK {
		t.Fatal("expected failed scan")
	}
	if len(st.Findings) != 1 || st.Findings[0].Title != "Scan did not finish" {
		t.Fatalf("findings %#v summary %q", st.Findings, st.Summary)
	}
	if strings.Contains(st.Findings[0].Detail, "Marco") {
		t.Fatalf("banner leaked: %q", st.Findings[0].Detail)
	}
}

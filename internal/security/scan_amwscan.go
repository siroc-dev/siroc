package security

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
)

type amwScanReport struct {
	Directory string          `json:"directory"`
	Scanned   json.RawMessage `json:"scanned"`
	Detected  json.RawMessage `json:"detected"`
	Infected  json.RawMessage `json:"infected"`
	Files     json.RawMessage `json:"files"`
}

type amwScanFile struct {
	File     string          `json:"file"`
	Path     string          `json:"path"`
	Status   string          `json:"status"`
	Severity json.RawMessage `json:"severity"`
	Exploits []string        `json:"exploits"`
	Reason   string          `json:"reason"`
	Mode     string          `json:"mode"`
}

func summarizeAMWScan(resp *rpc.ScanResp, body string) {
	resp.Title = "PHP antimalware scan"
	rep, err := parseAMWScanJSON(body)
	if err != nil || rep == nil {
		resp.Summary = "AMWScan finished. Open the original report for the raw log."
		if strings.TrimSpace(body) != "" {
			resp.Findings = append(resp.Findings, rpc.ScanFinding{Severity: "info", Title: "AMWScan output", Count: 1, Detail: clipWP(body)})
		}
		return
	}
	if rep.Directory != "" && resp.Target == "" {
		resp.Target = rep.Directory
	}
	n := 0
	for _, f := range amwScanFiles(rep) {
		path := strings.TrimSpace(f.File)
		if path == "" {
			path = strings.TrimSpace(f.Path)
		}
		if path == "" {
			continue
		}
		sev := amwScanSeverity(f)
		title := path
		if st := strings.TrimSpace(f.Status); st != "" && !strings.EqualFold(st, "infected") {
			title = st + " · " + path
		}
		detail := strings.Join(f.Exploits, ", ")
		if f.Reason != "" {
			if detail != "" {
				detail += " · "
			}
			detail += f.Reason
		}
		if f.Mode != "" && detail == "" {
			detail = f.Mode
		}
		resp.Findings = append(resp.Findings, rpc.ScanFinding{Severity: sev, Title: title, Count: 1, Detail: clipWP(detail)})
		n++
	}
	if n == 0 {
		resp.Summary = "No malicious PHP was reported."
		return
	}
	resp.Summary = ""
}

func parseAMWScanJSON(body string) (*amwScanReport, error) {
	s := strings.TrimSpace(body)
	if i := strings.Index(s, "{"); i > 0 {
		s = s[i:]
	}
	if s == "" || s[0] != '{' {
		return nil, fmt.Errorf("amwscan json")
	}
	var rep amwScanReport
	if err := json.Unmarshal([]byte(s), &rep); err != nil {
		return nil, err
	}
	return &rep, nil
}

func amwScanFiles(rep *amwScanReport) []amwScanFile {
	if rep == nil || len(rep.Files) == 0 {
		return nil
	}
	raw := strings.TrimSpace(string(rep.Files))
	if strings.HasPrefix(raw, "[") {
		var list []amwScanFile
		if json.Unmarshal(rep.Files, &list) == nil {
			return list
		}
	}
	var obj map[string]amwScanFile
	if json.Unmarshal(rep.Files, &obj) != nil {
		return nil
	}
	out := make([]amwScanFile, 0, len(obj))
	for path, f := range obj {
		if strings.TrimSpace(f.File) == "" && strings.TrimSpace(f.Path) == "" {
			f.File = path
		}
		out = append(out, f)
	}
	return out
}

func amwScanSeverity(f amwScanFile) string {
	st := strings.ToLower(strings.TrimSpace(f.Status))
	lab := strings.ToLower(strings.TrimSpace(rawJSONString(f.Severity)))
	n, _ := strconv.Atoi(strings.TrimSpace(string(f.Severity)))
	switch {
	case strings.Contains(lab, "keyword") || st == "warning" || lab == "info":
		return "info"
	case lab == "low" || n == 1:
		return "low"
	case lab == "medium" || st == "suspicious" || n == 2:
		return "medium"
	case lab == "critical" || lab == "high" || st == "infected" || n >= 3:
		return "high"
	case st == "detected" || st == "malware":
		return "high"
	default:
		if len(f.Exploits) > 0 {
			return "high"
		}
		return "medium"
	}
}

func rawJSONString(b json.RawMessage) string {
	s := strings.TrimSpace(string(b))
	s = strings.Trim(s, `"`)
	return s
}

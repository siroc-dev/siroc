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
	Path      string          `json:"path"`
	Scanned   json.RawMessage `json:"scanned"`
	Detected  json.RawMessage `json:"detected"`
	Infected  json.RawMessage `json:"infected"`
	Files     json.RawMessage `json:"files"`
	Results   json.RawMessage `json:"results"`
	Findings  json.RawMessage `json:"findings"`
}

type amwCanonicalFinding struct {
	Kind     string `json:"kind"`
	RuleID   string `json:"rule_id"`
	Subject  string `json:"subject"`
	Path     string `json:"path"`
	File     string `json:"file"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Status   string `json:"status"`
	Reason   string `json:"reason"`
}

type amwScanFile struct {
	File     string          `json:"file"`
	Path     string          `json:"path"`
	Status   string          `json:"status"`
	Severity json.RawMessage `json:"severity"`
	Exploits []string        `json:"exploits"`
	Reason   string          `json:"reason"`
	Mode     string          `json:"mode"`
	Type     string          `json:"type"`
	Key      string          `json:"key"`
	Level    string          `json:"level"`
	Line     json.RawMessage `json:"line"`
	Match    string          `json:"match"`
}

func summarizeAMWScan(resp *rpc.ScanResp, body string) {
	resp.Title = "PHP antimalware scan"
	rep, err := parseAMWScanJSON(body)
	if err != nil || rep == nil {
		if rows := amwScanTextRows(body); len(rows) > 0 {
			resp.Findings = append(resp.Findings, rows...)
			resp.Summary = ""
			return
		}
		resp.Summary = "AMWScan finished. Open the original report for the raw log."
		if strings.TrimSpace(body) != "" {
			resp.Findings = append(resp.Findings, rpc.ScanFinding{Severity: "info", Title: "AMWScan output", Count: 1, Detail: clipWP(body)})
		}
		return
	}
	if dir := strings.TrimSpace(rep.Directory); dir != "" && resp.Target == "" {
		resp.Target = dir
	} else if p := strings.TrimSpace(rep.Path); p != "" && resp.Target == "" {
		resp.Target = p
	}
	n := 0
	for _, f := range amwCanonicalRows(rep) {
		resp.Findings = append(resp.Findings, f)
		n++
	}
	if n == 0 {
		for _, f := range amwScanFiles(rep) {
			path := strings.TrimSpace(f.File)
			if path == "" {
				path = strings.TrimSpace(f.Path)
			}
			if path == "" {
				continue
			}
			resp.Findings = append(resp.Findings, amwFileFinding(path, f))
			n++
		}
	}
	if n == 0 {
		if rows := amwScanTextRows(body); len(rows) > 0 {
			resp.Findings = append(resp.Findings, rows...)
			resp.Summary = ""
			return
		}
		resp.Summary = "No malicious PHP was reported."
		return
	}
	resp.Summary = ""
}

func amwFileFinding(path string, f amwScanFile) rpc.ScanFinding {
	sev := amwScanSeverity(f)
	title := path
	if st := strings.TrimSpace(f.Status); st != "" && !strings.EqualFold(st, "infected") {
		title = st + " · " + path
	}
	if f.Type != "" && f.Key != "" {
		title = f.Type + " " + f.Key + " · " + path
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
	if line := strings.TrimSpace(rawJSONString(f.Line)); line != "" && line != "null" {
		if detail != "" {
			detail += " · "
		}
		detail += "line " + line
	}
	if f.Match != "" && detail == "" {
		detail = clipWP(f.Match)
	}
	return rpc.ScanFinding{Severity: sev, Title: title, Count: 1, Detail: clipWP(detail)}
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

func amwCanonicalRows(rep *amwScanReport) []rpc.ScanFinding {
	if rep == nil {
		return nil
	}
	var out []rpc.ScanFinding
	for _, raw := range []json.RawMessage{rep.Findings, rep.Detected, rep.Infected} {
		for _, f := range amwFindingList(raw) {
			path := strings.TrimSpace(f.Subject)
			if path == "" {
				path = strings.TrimSpace(f.Path)
			}
			if path == "" {
				path = strings.TrimSpace(f.File)
			}
			if path == "" {
				continue
			}
			title := path
			if rid := strings.TrimSpace(f.RuleID); rid != "" {
				title = rid + " · " + path
			} else if k := strings.TrimSpace(f.Kind); k != "" && !strings.EqualFold(k, "malware") {
				title = k + " · " + path
			}
			detail := strings.TrimSpace(f.Message)
			if detail == "" {
				detail = strings.TrimSpace(f.Reason)
			}
			out = append(out, rpc.ScanFinding{Severity: amwCanonSeverity(f), Title: title, Count: 1, Detail: clipWP(detail)})
		}
	}
	return out
}

func amwFindingList(raw json.RawMessage) []amwCanonicalFinding {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return nil
	}
	if strings.HasPrefix(s, "[") {
		var list []amwCanonicalFinding
		if json.Unmarshal(raw, &list) == nil {
			return list
		}
	}
	var obj map[string]amwCanonicalFinding
	if json.Unmarshal(raw, &obj) != nil {
		return nil
	}
	out := make([]amwCanonicalFinding, 0, len(obj))
	for path, f := range obj {
		if strings.TrimSpace(f.Subject) == "" && strings.TrimSpace(f.Path) == "" && strings.TrimSpace(f.File) == "" {
			f.Subject = path
		}
		out = append(out, f)
	}
	return out
}

func amwScanFiles(rep *amwScanReport) []amwScanFile {
	if rep == nil {
		return nil
	}
	if list := amwFileList(rep.Files); len(list) > 0 {
		return list
	}
	return amwFileList(rep.Results)
}

func amwFileList(raw json.RawMessage) []amwScanFile {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return nil
	}
	if strings.HasPrefix(s, "[") {
		var list []amwScanFile
		if json.Unmarshal(raw, &list) == nil {
			return list
		}
	}
	var obj map[string]amwScanFile
	if json.Unmarshal(raw, &obj) == nil {
		out := make([]amwScanFile, 0, len(obj))
		for path, f := range obj {
			if strings.TrimSpace(f.File) == "" && strings.TrimSpace(f.Path) == "" {
				f.File = path
			}
			out = append(out, f)
		}
		return out
	}
	var nested map[string][]amwScanFile
	if json.Unmarshal(raw, &nested) != nil {
		return nil
	}
	var out []amwScanFile
	for path, items := range nested {
		if len(items) == 0 {
			out = append(out, amwScanFile{File: path, Status: "infected"})
			continue
		}
		for _, f := range items {
			if strings.TrimSpace(f.File) == "" && strings.TrimSpace(f.Path) == "" {
				f.File = path
			}
			out = append(out, f)
		}
	}
	return out
}

func amwScanTextRows(body string) []rpc.ScanFinding {
	var out []rpc.ScanFinding
	cur := ""
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		low := strings.ToLower(t)
		if strings.HasPrefix(low, "file:") {
			cur = strings.TrimSpace(t[5:])
			continue
		}
		if strings.HasPrefix(t, "- ") && cur != "" {
			item := strings.TrimSpace(t[2:])
			sev := "medium"
			if strings.Contains(low, "danger") || strings.Contains(low, "malware") {
				sev = "high"
			} else if strings.Contains(low, "warning") || strings.Contains(low, "keyword") {
				sev = "info"
			}
			out = append(out, rpc.ScanFinding{Severity: sev, Title: item + " · " + cur, Count: 1})
		}
	}
	return out
}

func amwCanonSeverity(f amwCanonicalFinding) string {
	return amwScanSeverity(amwScanFile{Status: f.Status, Severity: json.RawMessage(strconv.Quote(f.Severity)), Reason: f.Kind + " " + f.Message})
}

func amwScanSeverity(f amwScanFile) string {
	st := strings.ToLower(strings.TrimSpace(f.Status))
	lab := strings.ToLower(strings.TrimSpace(rawJSONString(f.Severity)))
	if lab == "" {
		lab = strings.ToLower(strings.TrimSpace(f.Level))
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(f.Severity)))
	switch {
	case strings.Contains(lab, "keyword") || st == "warning" || lab == "info" || lab == "warning":
		return "info"
	case lab == "low" || n == 1:
		return "low"
	case lab == "medium" || st == "suspicious" || n == 2:
		return "medium"
	case lab == "critical" || lab == "high" || lab == "danger" || lab == "dangerous" || st == "infected" || n >= 3:
		return "high"
	case st == "detected" || st == "malware" || lab == "malware":
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

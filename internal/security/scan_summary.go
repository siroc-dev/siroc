//go:build linux

package security

import (
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func enrichScan(resp *rpc.ScanResp, raw string) {
	if resp == nil {
		return
	}
	if resp.CreatedAt == "" {
		resp.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	body := raw
	if strings.TrimSpace(body) == "" {
		body = resp.Output
	}
	switch resp.Tool {
	case "nikto":
		summarizeNikto(resp, body)
	case "zap":
		summarizeZAP(resp, body)
	case "openvas":
		summarizeOpenVAS(resp, body)
	default:
		if resp.Title == "" {
			resp.Title = "Scan complete"
		}
		if resp.Summary == "" {
			resp.Summary = "See the technical log for details."
		}
	}
	countFindings(resp)
	saveScanLog(resp)
}

func countFindings(resp *rpc.ScanResp) {
	resp.High, resp.Medium, resp.Low, resp.Info = 0, 0, 0, 0
	for _, f := range resp.Findings {
		n := f.Count
		if n < 1 {
			n = 1
		}
		switch f.Severity {
		case "high":
			resp.High += n
		case "medium":
			resp.Medium += n
		case "low":
			resp.Low += n
		default:
			resp.Info += n
		}
	}
	if resp.Title == "" {
		resp.Title = toolTitle(resp.Tool) + " scan"
	}
	if resp.Summary == "" {
		parts := filterEmpty([]string{
			countPhrase(resp.High, "high"),
			countPhrase(resp.Medium, "medium"),
			countPhrase(resp.Low, "low"),
			countPhrase(resp.Info, "info"),
		})
		if len(parts) == 0 {
			resp.Summary = "No issues were reported."
		} else {
			resp.Summary = "Found " + strings.Join(parts, ", ") + "."
		}
	}
}

func countPhrase(n int, label string) string {
	if n <= 0 {
		return ""
	}
	if n == 1 {
		return "1 " + label
	}
	return strconv.Itoa(n) + " " + label
}

func filterEmpty(in []string) []string {
	out := in[:0]
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func toolTitle(id string) string {
	switch id {
	case "nikto":
		return "Nikto"
	case "zap":
		return "OWASP ZAP"
	case "openvas":
		return "OpenVAS / Greenbone"
	default:
		return id
	}
}

func summarizeNikto(resp *rpc.ScanResp, body string) {
	groups := map[string]*rpc.ScanFinding{}
	order := []string{}
	host, port := "", ""
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "+ Target Host:") {
			host = strings.TrimSpace(strings.TrimPrefix(line, "+ Target Host:"))
			continue
		}
		if strings.HasPrefix(line, "+ Target Port:") {
			port = strings.TrimSpace(strings.TrimPrefix(line, "+ Target Port:"))
			continue
		}
		if !strings.HasPrefix(line, "+") {
			continue
		}
		line = strings.TrimPrefix(line, "+ ")
		low := strings.ToLower(line)
		if strings.HasPrefix(low, "target ") || strings.HasPrefix(low, "start time") || strings.HasPrefix(low, "end time") || strings.HasPrefix(low, "server:") || strings.Contains(low, "no cgi directories") {
			continue
		}
		title, sev, detail := classifyNikto(line)
		if f, ok := groups[title]; ok {
			f.Count++
			continue
		}
		groups[title] = &rpc.ScanFinding{Severity: sev, Title: title, Count: 1, Detail: detail}
		order = append(order, title)
	}
	for _, k := range order {
		resp.Findings = append(resp.Findings, *groups[k])
	}
	if resp.Target == "" && host != "" {
		resp.Target = host
		if port != "" && port != "80" {
			resp.Target += ":" + port
		}
	}
	resp.Title = "Website check (Nikto)"
	if len(resp.Findings) == 0 {
		resp.Summary = "Nikto finished and did not report extra issues beyond server info."
		return
	}
	resp.Summary = ""
}

func classifyNikto(line string) (title, sev, detail string) {
	detail = clipScan(line)
	l := strings.ToLower(line)
	switch {
	case strings.Contains(l, "x-frame-options"):
		return "The site can be embedded in other pages (missing X-Frame-Options)", "medium", "Add an X-Frame-Options or Content-Security-Policy frame-ancestors header."
	case strings.Contains(l, "x-content-type"):
		return "Browsers may guess file types (missing X-Content-Type-Options)", "low", "Add X-Content-Type-Options: nosniff."
	case strings.Contains(l, "strict-transport"):
		return "HTTPS is not locked in (missing HSTS)", "medium", "Add Strict-Transport-Security on HTTPS."
	case strings.Contains(l, "content-security-policy"):
		return "No Content-Security-Policy header", "medium", "Add a CSP header to reduce XSS impact."
	case strings.Contains(l, "server-status"):
		return "Apache server-status page is public", "high", "Disable or restrict /server-status in Apache."
	case strings.Contains(l, "server-info"):
		return "Apache server-info page is public", "high", "Disable or restrict /server-info."
	case strings.Contains(l, "php reveals") || strings.Contains(l, "phpinfo") || strings.Contains(l, "php credits"):
		return "PHP is exposing version details", "medium", "Turn off expose_php and remove phpinfo pages."
	case strings.Contains(l, "rfi from") || strings.Contains(l, "remote file inclusion") || strings.Contains(l, "rfiinc.txt"):
		return "Remote file inclusion style URLs were probed", "low", "Many of these are generic probes. Confirm the app does not include remote files from query strings."
	case strings.Contains(l, "directory indexing") || strings.Contains(l, "index of /"):
		return "Folder listing is enabled", "medium", "Disable autoindex so visitors cannot browse files."
	case strings.Contains(l, "track") || strings.Contains(l, "trace"):
		return "TRACE/TRACK HTTP method is enabled", "medium", "Disable TRACE/TRACK on the web server."
	case strings.Contains(l, "debug http"):
		return "HTTP DEBUG method is enabled", "low", "Disable the DEBUG method unless you need it."
	case strings.Contains(l, "osvdb") || strings.Contains(l, "cve-"):
		return "A known vulnerable path or product may be present", "medium", detail
	case strings.Contains(l, "allowed http method"):
		return "The server lists extra HTTP methods", "info", detail
	case strings.Contains(l, "uncommon header"):
		return "Uncommon HTTP headers were returned", "info", detail
	case strings.Contains(l, "cookie") && (strings.Contains(l, "httponly") || strings.Contains(l, "secure")):
		return "Cookies are missing Secure or HttpOnly flags", "medium", "Set Secure and HttpOnly on session cookies."
	default:
		title = line
		if i := strings.Index(title, ": "); i > 0 && i < 60 {
			rest := strings.TrimSpace(title[i+2:])
			if len(rest) > 12 {
				title = rest
			}
		}
		title = stripTags(title)
		if len(title) > 110 {
			title = title[:107] + "…"
		}
		return title, "info", detail
	}
}

func summarizeZAP(resp *rpc.ScanResp, body string) {
	resp.Title = "Website check (OWASP ZAP)"
	if zapAttackFailed(body) {
		resp.OK = false
		resp.Summary = zapFailSummary(body)
		resp.Findings = []rpc.ScanFinding{{
			Severity: "info",
			Title:    "Scan did not reach the site",
			Detail:   resp.Summary,
			Count:    1,
		}}
		return
	}
	groups := map[string]*rpc.ScanFinding{}
	order := []string{}
	alertRe := regexp.MustCompile(`(?is)<tr>\s*<td>\s*(?:<a[^>]*>)?([^<]+)(?:</a>)?\s*</td>\s*<td[^>]*class="risk-(-?\d)"[^>]*>\s*([^<]+)\s*</td>\s*<td[^>]*>\s*(\d+)\s*</td>`)
	for _, m := range alertRe.FindAllStringSubmatch(body, -1) {
		title := strings.TrimSpace(html.UnescapeString(m[1]))
		if title == "" || strings.EqualFold(title, "name") {
			continue
		}
		n := 1
		if v := atoiSafe(m[4]); v > 0 {
			n = v
		}
		sev := zapRiskLevel(m[2], m[3])
		if sev == "skip" {
			continue
		}
		if f, ok := groups[title]; ok {
			f.Count += n
			continue
		}
		groups[title] = &rpc.ScanFinding{Severity: zapSeverity(title, sev), Title: humanZAP(title), Count: n}
		order = append(order, title)
	}
	if len(order) == 0 {
		re := regexp.MustCompile(`(?m)^(WARN-NEW|WARN|FAIL-NEW|FAIL|INFO): (.+?)(?:\s+\[(\d+)\])?(?:\s+x\s+(\d+))?\s*$`)
		for _, m := range re.FindAllStringSubmatch(body, -1) {
			kind := strings.ToUpper(m[1])
			title := strings.TrimSpace(m[2])
			n := 1
			if m[4] != "" {
				if v := atoiSafe(m[4]); v > 0 {
					n = v
				}
			}
			sev := "medium"
			switch {
			case strings.HasPrefix(kind, "FAIL"):
				sev = "high"
			case strings.HasPrefix(kind, "INFO"):
				sev = "info"
			}
			if f, ok := groups[title]; ok {
				f.Count += n
				continue
			}
			groups[title] = &rpc.ScanFinding{Severity: zapSeverity(title, sev), Title: humanZAP(title), Count: n}
			order = append(order, title)
		}
	}
	for _, k := range order {
		resp.Findings = append(resp.Findings, *groups[k])
	}
	if len(resp.Findings) == 0 {
		resp.Summary = "ZAP finished and did not list extra alerts in this view."
	}
}

func zapRiskLevel(code, label string) string {
	switch strings.TrimSpace(code) {
	case "3":
		return "high"
	case "2":
		return "medium"
	case "1":
		return "low"
	case "0":
		return "info"
	case "-1":
		return "skip"
	}
	l := strings.ToLower(label)
	switch {
	case strings.Contains(l, "high"):
		return "high"
	case strings.Contains(l, "medium"):
		return "medium"
	case strings.Contains(l, "low"):
		return "low"
	case strings.Contains(l, "false"):
		return "skip"
	default:
		return "info"
	}
}

func zapSeverity(title, fallback string) string {
	l := strings.ToLower(title)
	switch {
	case strings.Contains(l, "sql injection") || strings.Contains(l, "remote os command") || strings.Contains(l, "path traversal") || strings.Contains(l, "remote file inclusion") || strings.Contains(l, "server-status"):
		return "high"
	case strings.Contains(l, "xss") || strings.Contains(l, "cross site") || strings.Contains(l, "clickjack") || strings.Contains(l, "csp") || strings.Contains(l, "https"):
		return "medium"
	case strings.Contains(l, "x-content-type") || strings.Contains(l, "cache-control") || strings.Contains(l, "server leaks") || strings.Contains(l, "timestamp"):
		return "low"
	default:
		return fallback
	}
}

func humanZAP(title string) string {
	l := strings.ToLower(title)
	switch {
	case strings.Contains(l, "anti-clickjacking") || strings.Contains(l, "x-frame-options"):
		return "The site can be embedded in other pages (clickjacking)"
	case strings.Contains(l, "content security policy"):
		return "No Content-Security-Policy header"
	case strings.Contains(l, "x-content-type"):
		return "Browsers may guess file types (MIME sniffing)"
	case strings.Contains(l, "strict-transport"):
		return "HTTPS is not locked in (missing HSTS)"
	case strings.Contains(l, "server leaks information"):
		return "Server version is visible in HTTP headers"
	case strings.Contains(l, "absence of anti-csrf"):
		return "Forms may be missing CSRF protection"
	default:
		return title
	}
}

func summarizeOpenVAS(resp *rpc.ScanResp, body string) {
	resp.Title = "OpenVAS / Greenbone scan"
	low := strings.ToLower(body)
	if strings.Contains(low, "not ready") || strings.Contains(low, "socket not found") {
		resp.OK = false
		resp.Summary = "Greenbone is not ready yet. Feeds may still be downloading, or the services are stopped."
		resp.Findings = []rpc.ScanFinding{{Severity: "info", Title: "Scanner not ready", Detail: clipScan(body), Count: 1}}
		return
	}
	resp.Summary = "A Greenbone task was started for this website. Results appear in Scan logs when the scan finishes. You can also open Greenbone at port 9392."
	resp.Findings = []rpc.ScanFinding{
		{Severity: "info", Title: "Task started in Greenbone", Detail: "This scanner runs in the background. Check Scan logs or the Greenbone web UI for the full report.", Count: 1},
	}
}

func atoiSafe(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func stripTags(s string) string {
	re := regexp.MustCompile(`<[^>]+>`)
	return strings.TrimSpace(re.ReplaceAllString(s, ""))
}

func saveScanLog(resp *rpc.ScanResp) {
	if resp == nil || resp.ID == "" {
		return
	}
	_ = os.MkdirAll(scanDir, 0750)
	copy := *resp
	if len(copy.Output) > 20000 {
		copy.Output = copy.Output[len(copy.Output)-20000:]
	}
	raw, err := json.MarshalIndent(copy, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(scanDir, resp.ID+".json"), raw, 0640)
}

func (m *Manager) ListScanLogs() []rpc.ScanResp {
	ents, err := os.ReadDir(scanDir)
	if err != nil {
		return []rpc.ScanResp{}
	}
	seen := map[string]bool{}
	var out []rpc.ScanResp
	for _, e := range ents {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		if st, err := loadScanLog(id); err == nil {
			refreshZapLog(st)
			seen[id] = true
			out = append(out, *st)
		}
	}
	for _, e := range ents {
		name := e.Name()
		ext := filepath.Ext(name)
		if ext != ".txt" && ext != ".html" && ext != ".xml" {
			continue
		}
		id := strings.TrimSuffix(name, ext)
		if seen[id] || !scanIDOK(id) {
			continue
		}
		st := scanFromReportFile(id, filepath.Join(scanDir, name))
		seen[id] = true
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

func (m *Manager) GetScanLog(id string) (*rpc.ScanResp, error) {
	if !scanIDOK(id) {
		return nil, os.ErrNotExist
	}
	if st, err := loadScanLog(id); err == nil {
		refreshZapLog(st)
		return st, nil
	}
	_, body, err := m.ScanReport(id)
	if err != nil {
		return nil, err
	}
	st := scanFromReportFile(id, "")
	st.Output = clipScan(string(body))
	enrichScan(&st, string(body))
	return &st, nil
}

func loadScanLog(id string) (*rpc.ScanResp, error) {
	b, err := os.ReadFile(filepath.Join(scanDir, id+".json"))
	if err != nil {
		return nil, err
	}
	var st rpc.ScanResp
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func scanFromReportFile(id, path string) rpc.ScanResp {
	tool := "scan"
	for _, t := range []string{"nikto", "zap", "openvas"} {
		if strings.Contains(id, t) {
			tool = t
			break
		}
	}
	st := rpc.ScanResp{OK: true, ID: id, Tool: tool, Report: "/api/security/scans/" + id}
	if path != "" {
		if b, err := os.ReadFile(path); err == nil {
			st.Output = clipScan(string(b))
			created := fileTime(path)
			if created != "" {
				st.CreatedAt = created
			}
			tmp := st
			enrichScan(&tmp, string(b))
			tmp.ID = id
			tmp.Tool = tool
			tmp.Report = st.Report
			return tmp
		}
	}
	return st
}

func zapFindingsLookLikeHeadings(findings []rpc.ScanFinding) bool {
	for _, f := range findings {
		l := strings.ToLower(f.Title)
		if strings.Contains(l, "summary of alerts") || strings.Contains(l, "zap version") || strings.HasPrefix(l, "generated on") || strings.EqualFold(f.Title, "Alerts") || strings.EqualFold(f.Title, "Alert Detail") {
			return true
		}
	}
	return false
}

func refreshZapLog(st *rpc.ScanResp) {
	if st == nil || st.Tool != "zap" {
		return
	}
	if !zapFindingsLookLikeHeadings(st.Findings) && !zapAttackFailed(st.Output) {
		return
	}
	st.Output = zapCleanLog(st.Output)
	htmlPath := filepath.Join(scanDir, st.ID+".html")
	b, err := os.ReadFile(htmlPath)
	if err != nil {
		if zapAttackFailed(st.Output) {
			st.Findings = nil
			st.Summary = ""
			st.Title = ""
			enrichScan(st, st.Output)
		}
		return
	}
	st.Findings = nil
	st.Summary = ""
	st.Title = ""
	enrichScan(st, string(b)+"\n"+st.Output)
}

func fileTime(path string) string {
	st, err := os.Stat(path)
	if err != nil {
		return ""
	}
	return st.ModTime().UTC().Format(time.RFC3339)
}

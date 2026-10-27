package security

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
)

type wpScanReport struct {
	ScanAborted         string                     `json:"scan_aborted"`
	TargetURL           string                     `json:"target_url"`
	Version             *wpScanComponent           `json:"version"`
	MainTheme           *wpScanComponent           `json:"main_theme"`
	Plugins             map[string]wpScanComponent `json:"plugins"`
	Themes              map[string]wpScanComponent `json:"themes"`
	Users               map[string]json.RawMessage `json:"users"`
	InterestingFindings []wpScanNote               `json:"interesting_findings"`
}

type wpScanComponent struct {
	Slug            string       `json:"slug"`
	Number          string       `json:"number"`
	Status          string       `json:"status"`
	Outdated        bool         `json:"outdated"`
	LatestVersion   string       `json:"latest_version"`
	Vulnerabilities []wpScanVuln `json:"vulnerabilities"`
}

type wpScanVuln struct {
	Title      string `json:"title"`
	FixedIn    string `json:"fixed_in"`
	References struct {
		CVE []string `json:"cve"`
		URL []string `json:"url"`
	} `json:"references"`
}

type wpScanNote struct {
	ToS                string   `json:"to_s"`
	Type               string   `json:"type"`
	InterestingEntries []string `json:"interesting_entries"`
}

func summarizeWPScan(resp *rpc.ScanResp, body string) {
	resp.Title = "WordPress check (WPScan)"
	rep, err := parseWPScanJSON(body)
	if err != nil || rep == nil {
		resp.Summary = "WPScan finished. Open the original report for the raw log."
		if strings.TrimSpace(body) != "" {
			resp.Findings = append(resp.Findings, rpc.ScanFinding{Severity: "info", Title: "WPScan output", Count: 1, Detail: clipWP(body)})
		}
		return
	}
	if rep.TargetURL != "" && resp.Target == "" {
		resp.Target = rep.TargetURL
	}
	if strings.TrimSpace(rep.ScanAborted) != "" {
		resp.OK = false
		resp.Summary = "WPScan stopped: " + clipWP(rep.ScanAborted)
		resp.Findings = append(resp.Findings, rpc.ScanFinding{Severity: "high", Title: "Scan aborted", Count: 1, Detail: clipWP(rep.ScanAborted)})
		return
	}
	addWPScanComponent(resp, "WordPress core", "wordpress", rep.Version)
	addWPScanComponent(resp, "Theme", themeName(rep.MainTheme), rep.MainTheme)
	for slug, p := range rep.Plugins {
		name := p.Slug
		if name == "" {
			name = slug
		}
		addWPScanComponent(resp, "Plugin", name, &p)
	}
	for slug, th := range rep.Themes {
		name := th.Slug
		if name == "" {
			name = slug
		}
		if rep.MainTheme != nil && (name == rep.MainTheme.Slug || slug == rep.MainTheme.Slug) {
			continue
		}
		addWPScanComponent(resp, "Theme", name, &th)
	}
	if n := len(rep.Users); n > 0 {
		names := make([]string, 0, n)
		for login := range rep.Users {
			names = append(names, login)
		}
		resp.Findings = append(resp.Findings, rpc.ScanFinding{
			Severity: "low",
			Title:    "WordPress usernames enumerated",
			Count:    n,
			Detail:   strings.Join(names, ", "),
		})
	}
	for _, note := range rep.InterestingFindings {
		title := strings.TrimSpace(note.ToS)
		if title == "" {
			title = strings.TrimSpace(note.Type)
		}
		if title == "" {
			continue
		}
		detail := strings.Join(note.InterestingEntries, "; ")
		resp.Findings = append(resp.Findings, rpc.ScanFinding{Severity: "info", Title: title, Count: 1, Detail: clipWP(detail)})
	}
	resp.Summary = ""
}

func parseWPScanJSON(body string) (*wpScanReport, error) {
	s := strings.TrimSpace(body)
	if i := strings.Index(s, "{"); i > 0 {
		s = s[i:]
	}
	if s == "" || s[0] != '{' {
		return nil, fmt.Errorf("wpscan json")
	}
	var rep wpScanReport
	if err := json.Unmarshal([]byte(s), &rep); err != nil {
		return nil, err
	}
	return &rep, nil
}

func clipWP(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 800 {
		return s[:800]
	}
	return s
}

func themeName(c *wpScanComponent) string {
	if c == nil {
		return "theme"
	}
	if c.Slug != "" {
		return c.Slug
	}
	return "theme"
}

func addWPScanComponent(resp *rpc.ScanResp, kind, name string, c *wpScanComponent) {
	if c == nil {
		return
	}
	ver := strings.TrimSpace(c.Number)
	label := kind + " " + name
	if ver != "" {
		label += " " + ver
	}
	for _, v := range c.Vulnerabilities {
		title := strings.TrimSpace(v.Title)
		if title == "" {
			title = label + " vulnerability"
		}
		detail := label
		if v.FixedIn != "" {
			detail += " · fixed in " + v.FixedIn
		}
		if len(v.References.CVE) > 0 {
			detail += " · CVE " + strings.Join(v.References.CVE, ", ")
		}
		resp.Findings = append(resp.Findings, rpc.ScanFinding{Severity: "high", Title: title, Count: 1, Detail: clipWP(detail)})
	}
	if c.Outdated || strings.EqualFold(c.Status, "outdated") {
		detail := label + " is outdated"
		if c.LatestVersion != "" {
			detail += " (latest " + c.LatestVersion + ")"
		}
		resp.Findings = append(resp.Findings, rpc.ScanFinding{Severity: "medium", Title: label + " is outdated", Count: 1, Detail: detail})
	}
}

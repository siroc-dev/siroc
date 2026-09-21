//go:build linux

package security

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
)

const (
	wafStateFile  = "/var/lib/siroc/waf.json"
	wafEngineFile = "/etc/modsecurity/cp-engine.conf"
	wafSetupFile  = "/etc/modsecurity/cp-crs-setup.conf"
	wafRulesFile  = "/etc/modsecurity/cp-rules.conf"
)

type wafState struct {
	Mode              string   `json:"mode"`
	Paranoia          int      `json:"paranoia"`
	InboundThreshold  int      `json:"inboundThreshold"`
	OutboundThreshold int      `json:"outboundThreshold"`
	Audit             string   `json:"audit"`
	Packs             []string `json:"packs"`
	DisabledIDs       []int    `json:"disabledIds"`
}

type wafPackSpec struct {
	ID          string
	Title       string
	Description string
	Prefixes    []string
}

func wafPackCatalog() []wafPackSpec {
	return []wafPackSpec{
		{ID: "scanner", Title: "Scanner detection", Description: "Block vulnerability scanners and bots", Prefixes: []string{"REQUEST-913"}},
		{ID: "protocol", Title: "Protocol enforcement", Description: "HTTP method, protocol, and multipart checks", Prefixes: []string{"REQUEST-911", "REQUEST-920", "REQUEST-921", "REQUEST-922"}},
		{ID: "lfi", Title: "Local file inclusion", Description: "Path traversal and LFI", Prefixes: []string{"REQUEST-930"}},
		{ID: "rfi", Title: "Remote file inclusion", Description: "Remote file include attacks", Prefixes: []string{"REQUEST-931"}},
		{ID: "rce", Title: "Remote code execution", Description: "OS command and code injection", Prefixes: []string{"REQUEST-932"}},
		{ID: "php", Title: "PHP attacks", Description: "PHP-specific exploits", Prefixes: []string{"REQUEST-933"}},
		{ID: "generic", Title: "Generic application attacks", Description: "Generic web application attacks", Prefixes: []string{"REQUEST-934"}},
		{ID: "xss", Title: "Cross-site scripting", Description: "XSS in requests", Prefixes: []string{"REQUEST-941"}},
		{ID: "sqli", Title: "SQL injection", Description: "SQLi in requests", Prefixes: []string{"REQUEST-942"}},
		{ID: "session", Title: "Session fixation", Description: "Session fixation attempts", Prefixes: []string{"REQUEST-943"}},
		{ID: "java", Title: "Java attacks", Description: "Java / Jakarta exploits", Prefixes: []string{"REQUEST-944"}},
		{ID: "nodejs", Title: "Node.js attacks", Description: "Node.js application attacks", Prefixes: []string{"REQUEST-934-APPLICATION-ATTACK-NODEJS", "REQUEST-948"}},
		{ID: "leakage", Title: "Data leakage", Description: "SQL / PHP / IIS information leaks in responses", Prefixes: []string{"RESPONSE-950", "RESPONSE-951", "RESPONSE-953", "RESPONSE-954"}},
	}
}

func defaultWAFState() wafState {
	return wafState{
		Mode:              "DetectionOnly",
		Paranoia:          1,
		InboundThreshold:  5,
		OutboundThreshold: 4,
		Audit:             "RelevantOnly",
		Packs:             []string{"scanner", "protocol", "lfi", "rfi", "rce", "php", "generic", "xss", "sqli", "session", "leakage"},
	}
}

func loadWAFState() wafState {
	st := defaultWAFState()
	b, err := os.ReadFile(wafStateFile)
	if err == nil {
		_ = json.Unmarshal(b, &st)
	}
	if st.Mode == "" {
		if b, err := os.ReadFile(wafEngineFile); err == nil {
			s := string(b)
			switch {
			case strings.Contains(s, "SecRuleEngine On"):
				st.Mode = "On"
			case strings.Contains(s, "SecRuleEngine DetectionOnly"):
				st.Mode = "DetectionOnly"
			default:
				st.Mode = "Off"
			}
		}
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
	return st
}

func (m *Manager) WAFStatus() (*rpc.WAFStatus, error) {
	out := &rpc.WAFStatus{Mode: "Off", Packs: []rpc.WAFPack{}}
	if ok, ver := dpkgOK("libapache2-mod-security2"); ok {
		out.Installed = true
		out.Version = ver
	}
	st := loadWAFState()
	out.Mode = st.Mode
	out.Enabled = st.Mode == "On" || st.Mode == "DetectionOnly"
	out.Paranoia = st.Paranoia
	out.InboundThreshold = st.InboundThreshold
	out.OutboundThreshold = st.OutboundThreshold
	out.Audit = st.Audit
	out.DisabledIDs = st.DisabledIDs
	if out.DisabledIDs == nil {
		out.DisabledIDs = []int{}
	}
	dir := crsRulesDir()
	out.CRS = dir != ""
	if !out.CRS && out.Installed {
		out.Message = "OWASP CRS is not installed. Reinstall ModSecurity WAF from Software."
	}
	enabled := map[string]bool{}
	for _, id := range st.Packs {
		enabled[id] = true
	}
	for _, p := range wafPackCatalog() {
		item := rpc.WAFPack{ID: p.ID, Title: p.Title, Description: p.Description, Enabled: enabled[p.ID], Available: dir != "" && len(matchRules(dir, p.Prefixes)) > 0}
		if dir == "" {
			item.Available = true
		}
		out.Packs = append(out.Packs, item)
	}
	return out, nil
}

func (m *Manager) WAFSetMode(mode string) error {
	return m.WAFApply(rpc.WAFModeReq{Mode: mode})
}

func (m *Manager) WAFApply(req rpc.WAFModeReq) error {
	st := loadWAFState()
	if req.Mode != "" {
		switch req.Mode {
		case "On", "DetectionOnly", "Off":
			st.Mode = req.Mode
		default:
			return fmt.Errorf("mode must be On, DetectionOnly, or Off")
		}
	}
	if req.Paranoia != 0 {
		if req.Paranoia < 1 || req.Paranoia > 4 {
			return fmt.Errorf("paranoia must be 1-4")
		}
		st.Paranoia = req.Paranoia
	}
	if req.InboundThreshold != 0 {
		if req.InboundThreshold < 1 || req.InboundThreshold > 100 {
			return fmt.Errorf("inbound threshold must be 1-100")
		}
		st.InboundThreshold = req.InboundThreshold
	}
	if req.OutboundThreshold != 0 {
		if req.OutboundThreshold < 1 || req.OutboundThreshold > 100 {
			return fmt.Errorf("outbound threshold must be 1-100")
		}
		st.OutboundThreshold = req.OutboundThreshold
	}
	if req.Audit != "" {
		switch req.Audit {
		case "On", "Off", "RelevantOnly":
			st.Audit = req.Audit
		default:
			return fmt.Errorf("audit must be Off, RelevantOnly, or On")
		}
	}
	if req.Packs != nil {
		allowed := map[string]bool{}
		for _, p := range wafPackCatalog() {
			allowed[p.ID] = true
		}
		var packs []string
		seen := map[string]bool{}
		for _, id := range req.Packs {
			if !allowed[id] {
				return fmt.Errorf("unknown rule pack %q", id)
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			packs = append(packs, id)
		}
		st.Packs = packs
	}
	if req.DisabledIDs != nil {
		ids, err := normalizeRuleIDs(req.DisabledIDs)
		if err != nil {
			return err
		}
		st.DisabledIDs = ids
	}
	return writeWAF(st)
}

func WriteDefaultWAF() error {
	if _, err := os.Stat(wafStateFile); err == nil {
		return writeWAF(loadWAFState())
	}
	return writeWAF(defaultWAFState())
}

func EnsureWAF() error {
	if ok, _ := dpkgOK("libapache2-mod-security2"); !ok {
		return nil
	}
	_ = os.MkdirAll("/var/cache/modsecurity", 0750)
	sec2 := `<IfModule security2_module>
    SecDataDir /var/cache/modsecurity
    IncludeOptional /etc/modsecurity/modsecurity.conf
    IncludeOptional /etc/modsecurity/cp-engine.conf
    IncludeOptional /etc/modsecurity/cp-crs-setup.conf
    IncludeOptional /etc/modsecurity/cp-rules.conf
</IfModule>
`
	if err := os.WriteFile("/etc/apache2/mods-available/security2.conf", []byte(sec2), 0644); err != nil {
		return err
	}
	return WriteDefaultWAF()
}

func writeWAF(st wafState) error {
	if err := os.MkdirAll("/etc/modsecurity", 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(wafStateFile), 0750); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(wafStateFile, raw, 0640); err != nil {
		return err
	}
	if err := os.WriteFile(wafEngineFile, []byte("SecRuleEngine "+st.Mode+"\n"), 0644); err != nil {
		return err
	}
	setup := fmt.Sprintf(`SecRequestBodyAccess On
SecResponseBodyAccess On
SecAuditEngine %s
SecAuditLog /var/log/apache2/modsec_audit.log
SecAuditLogParts ABIJDEFHZ
SecDefaultAction "phase:1,log,auditlog,pass"
SecDefaultAction "phase:2,log,auditlog,pass"
SecAction "id:900000,phase:1,nolog,pass,t:none,setvar:tx.blocking_paranoia_level=%d"
SecAction "id:900001,phase:1,nolog,pass,t:none,setvar:tx.detection_paranoia_level=%d"
SecAction "id:900110,phase:1,nolog,pass,t:none,setvar:tx.inbound_anomaly_score_threshold=%d"
SecAction "id:900111,phase:1,nolog,pass,t:none,setvar:tx.outbound_anomaly_score_threshold=%d"
SecAction "id:900990,phase:1,nolog,pass,t:none,setvar:tx.crs_setup_version=%d"
`, st.Audit, st.Paranoia, st.Paranoia, st.InboundThreshold, st.OutboundThreshold, defaultCRSSetupVersion())
	if err := os.WriteFile(wafSetupFile, []byte(setup), 0644); err != nil {
		return err
	}
	dir := crsRulesDir()
	var b strings.Builder
	if dir == "" {
		b.WriteString("# OWASP CRS rules directory not found\n")
	} else {
		seen := map[string]bool{}
		include := func(files []string) {
			for _, f := range files {
				if seen[f] {
					continue
				}
				seen[f] = true
				fmt.Fprintf(&b, "Include %s\n", f)
			}
		}
		for _, prefix := range []string{"REQUEST-900", "REQUEST-901", "REQUEST-905"} {
			include(matchRules(dir, []string{prefix}))
		}
		enabled := map[string]bool{}
		for _, id := range st.Packs {
			enabled[id] = true
		}
		for _, p := range wafPackCatalog() {
			if !enabled[p.ID] {
				continue
			}
			include(matchRules(dir, p.Prefixes))
		}
		for _, prefix := range []string{"REQUEST-949", "RESPONSE-959", "RESPONSE-980"} {
			include(matchRules(dir, []string{prefix}))
		}
		if len(st.DisabledIDs) > 0 {
			b.WriteString("\n# Disabled rule IDs\n")
			for _, id := range st.DisabledIDs {
				fmt.Fprintf(&b, "SecRuleRemoveById %d\n", id)
			}
		}
	}
	if err := os.WriteFile(wafRulesFile, []byte(b.String()), 0644); err != nil {
		return err
	}
	_ = exec.Command("systemctl", "reload", "apache2").Run()
	return nil
}

func crsRulesDir() string {
	for _, dir := range []string{
		"/usr/share/modsecurity-crs/rules",
		"/usr/share/modsecurity-crs/owasp-crs/rules",
		"/etc/modsecurity/crs/rules",
	} {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			return dir
		}
	}
	return ""
}

func matchRules(dir string, prefixes []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, prefix := range prefixes {
		matches, _ := filepath.Glob(filepath.Join(dir, prefix+"*"))
		for _, f := range matches {
			if strings.HasSuffix(f, ".example") || strings.Contains(f, ".disabled") {
				continue
			}
			if seen[f] {
				continue
			}
			seen[f] = true
			out = append(out, f)
		}
	}
	return out
}

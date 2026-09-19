//go:build linux

package security

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

type Manager struct {
	HomeRoot string
}

func (m *Manager) FirewallStatus() (*rpc.FirewallStatus, error) {
	st := &rpc.FirewallStatus{}
	if _, err := exec.LookPath("ufw"); err != nil {
		return st, nil
	}
	st.Installed = true
	out, err := exec.Command("ufw", "status", "numbered").CombinedOutput()
	if err != nil {
		st.Message = strings.TrimSpace(string(out))
		return st, nil
	}
	text := string(out)
	low := strings.ToLower(text)
	st.Active = strings.Contains(low, "status: active")
	if strings.Contains(low, "default: deny (incoming)") {
		st.DefaultIncoming = "deny"
	} else if strings.Contains(low, "default: allow (incoming)") {
		st.DefaultIncoming = "allow"
	}
	st.Rules = parseUFWRules(text)
	st.DefaultPorts = defaultPublicPorts()
	return st, nil
}

func defaultPublicPorts() []string {
	return []string{"22", "21", "80", "443", CPWebPort()}
}

func DefaultUFWAllows() []string {
	return []string{
		"22/tcp",
		"21/tcp",
		"80/tcp",
		"443/tcp",
		CPWebPort() + "/tcp",
		"30000:30100/tcp",
	}
}

func CPWebPort() string {
	addr := strings.TrimSpace(os.Getenv("SIROC_LISTEN"))
	if addr == "" {
		return "8443"
	}
	if _, p, err := net.SplitHostPort(addr); err == nil && p != "" {
		return p
	}
	return "8443"
}

func ApplyDefaultUFW() {
	for _, p := range DefaultUFWAllows() {
		if exec.Command("ufw", "allow", p, "comment", "siroc-default").Run() != nil {
			_ = exec.Command("ufw", "allow", p).Run()
		}
	}
}

func (m *Manager) FirewallEnable(enable bool) error {
	if enable {
		ApplyDefaultUFW()
		out, err := exec.Command("ufw", "--force", "enable").CombinedOutput()
		if err != nil {
			return fmt.Errorf("ufw enable: %s", strings.TrimSpace(string(out)))
		}
		return nil
	}
	out, err := exec.Command("ufw", "disable").CombinedOutput()
	if err != nil {
		return fmt.Errorf("ufw disable: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (m *Manager) FirewallAdd(req rpc.FirewallRuleReq) error {
	if err := validateRule(req); err != nil {
		return err
	}
	args := []string{req.Action}
	if req.Source != "" {
		args = append(args, "from", req.Source, "to", "any")
	}
	spec := req.Port
	if req.Proto != "" && req.Proto != "any" {
		spec = req.Port + "/" + req.Proto
	}
	args = append(args, spec)
	out, err := exec.Command("ufw", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ufw: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (m *Manager) FirewallDelete(id int) error {
	if id < 1 || id > 256 {
		return fmt.Errorf("invalid rule id")
	}
	cmd := exec.Command("ufw", "--force", "delete", strconv.Itoa(id))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ufw delete: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (m *Manager) AVStatus() (*rpc.AVStatus, error) {
	st := &rpc.AVStatus{}
	if ok, ver := dpkgOK("clamav"); ok {
		st.Installed = true
		st.Version = ver
	}
	st.DaemonActive = exec.Command("systemctl", "is-active", "--quiet", "clamav-daemon").Run() == nil
	st.FreshclamActive = exec.Command("systemctl", "is-active", "--quiet", "clamav-freshclam").Run() == nil
	if out, err := exec.Command("sigtool", "--info", "/var/lib/clamav/main.cvd").CombinedOutput(); err == nil {
		st.Signatures = firstLine(string(out))
	}
	if b, err := os.ReadFile("/var/lib/siroc/last-scan.txt"); err == nil {
		st.LastScan = strings.TrimSpace(string(b))
	}
	return st, nil
}

func (m *Manager) AVUpdate() error {
	_ = os.MkdirAll("/var/log/clamav", 0755)
	_ = exec.Command("chown", "-R", "clamav:clamav", "/var/log/clamav").Run()
	running := exec.Command("systemctl", "is-active", "--quiet", "clamav-freshclam").Run() == nil
	if running {
		if out, err := exec.Command("systemctl", "stop", "clamav-freshclam").CombinedOutput(); err != nil {
			return fmt.Errorf("stop freshclam: %s", strings.TrimSpace(string(out)))
		}
		defer exec.Command("systemctl", "start", "clamav-freshclam").Run()
	}
	cmd := exec.Command("freshclam", "--foreground", "--stdout")
	out, err := combinedTimeout(cmd, 6*time.Minute)
	if err != nil {
		msg := strings.TrimSpace(out)
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("freshclam: %s", tail(msg, 8))
	}
	if exec.Command("systemctl", "is-active", "--quiet", "clamav-daemon").Run() == nil {
		_ = exec.Command("systemctl", "reload", "clamav-daemon").Run()
	}
	return nil
}

func (m *Manager) AVScan(rel string) (*rpc.AVScanResp, error) {
	target, err := m.scanPath(rel)
	if err != nil {
		return nil, err
	}
	bin := "clamscan"
	args := []string{"-r", "--infected", "--max-filesize=25M", "--max-scansize=100M", "--bell=no", target}
	if _, err := exec.LookPath("clamdscan"); err == nil && exec.Command("systemctl", "is-active", "--quiet", "clamav-daemon").Run() == nil {
		bin = "clamdscan"
		args = []string{"--fdpass", "--infected", target}
	}
	cmd := exec.Command(bin, args...)
	out, err := combinedTimeout(cmd, 8*time.Minute)
	summary := tail(out, 20)
	infected := countInfected(out)
	_ = os.MkdirAll("/var/lib/siroc", 0750)
	_ = os.WriteFile("/var/lib/siroc/last-scan.txt", []byte(time.Now().UTC().Format(time.RFC3339)+" "+target+" infected="+strconv.Itoa(infected)+"\n"+summary), 0640)
	if err != nil && infected == 0 && !strings.Contains(out, "Infected files") {
		return nil, fmt.Errorf("%s: %s", bin, tail(out, 8))
	}
	return &rpc.AVScanResp{OK: infected == 0, Summary: summary, Infected: infected, Path: target}, nil
}

func (m *Manager) scanPath(rel string) (string, error) {
	if rel == "" || rel == "/" {
		return "/home", nil
	}
	abs, err := filepath.Abs(rel)
	if err != nil {
		return "", err
	}
	allowed := []string{"/home", "/var/www", "/tmp"}
	ok := false
	for _, root := range allowed {
		r, err := filepath.Rel(root, abs)
		if err == nil && !strings.HasPrefix(r, "..") {
			ok = true
			break
		}
	}
	if !ok {
		return "", fmt.Errorf("scan path must be under /home, /var/www, or /tmp")
	}
	return abs, nil
}

var (
	ruleLine = regexp.MustCompile(`^\[\s*(\d+)\]\s+(\S+)\s+(ALLOW|DENY|REJECT)\s+IN\s+(.+)$`)
	portRe   = regexp.MustCompile(`^[0-9]{1,5}(:[0-9]{1,5})?$`)
	cidrRe   = regexp.MustCompile(`^([0-9]{1,3}\.){3}[0-9]{1,3}(/([0-9]|[12][0-9]|3[0-2]))?$`)
)

func parseUFWRules(text string) []rpc.FirewallRule {
	var rules []rpc.FirewallRule
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		m := ruleLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		id, _ := strconv.Atoi(m[1])
		to := m[2]
		port, proto := to, "any"
		if i := strings.Index(to, "/"); i >= 0 {
			port, proto = to[:i], to[i+1:]
		}
		rules = append(rules, rpc.FirewallRule{
			ID:     id,
			Action: strings.ToLower(m[3]),
			Port:   port,
			Proto:  proto,
			Source: strings.TrimSpace(m[4]),
		})
	}
	if rules == nil {
		rules = []rpc.FirewallRule{}
	}
	return rules
}

func validateRule(req rpc.FirewallRuleReq) error {
	switch req.Action {
	case "allow", "deny":
	default:
		return fmt.Errorf("action must be allow or deny")
	}
	switch req.Proto {
	case "", "tcp", "udp", "any":
	default:
		return fmt.Errorf("protocol must be tcp, udp, or any")
	}
	if !portRe.MatchString(req.Port) {
		return fmt.Errorf("port must be a number or range like 8080:8090")
	}
	lo, hi := splitPort(req.Port)
	if lo < 1 || lo > 65535 || hi < 1 || hi > 65535 || lo > hi {
		return fmt.Errorf("port out of range")
	}
	if req.Source != "" && req.Source != "Anywhere" && !cidrRe.MatchString(req.Source) {
		return fmt.Errorf("source must be IPv4 or CIDR")
	}
	return nil
}

func splitPort(s string) (int, int) {
	parts := strings.Split(s, ":")
	a, _ := strconv.Atoi(parts[0])
	if len(parts) == 1 {
		return a, a
	}
	b, _ := strconv.Atoi(parts[1])
	return a, b
}

func dpkgOK(pkg string) (bool, string) {
	out, err := exec.Command("dpkg-query", "-W", "-f", "${Status}|${Version}", pkg).CombinedOutput()
	if err != nil {
		return false, ""
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), "|", 2)
	if len(parts) != 2 || !strings.Contains(parts[0], "install ok installed") {
		return false, ""
	}
	return true, parts[1]
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

func countInfected(out string) int {
	re := regexp.MustCompile(`Infected files:\s+(\d+)`)
	m := re.FindStringSubmatch(out)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

func combinedTimeout(cmd *exec.Cmd, d time.Duration) (string, error) {
	var buf strings.Builder
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Start(); err != nil {
		return "", err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return buf.String(), err
	case <-time.After(d):
		_ = cmd.Process.Kill()
		return buf.String(), fmt.Errorf("timed out after %s", d)
	}
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

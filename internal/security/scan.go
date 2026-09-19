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
	"github.com/siroc-dev/siroc/internal/validate"
)

const scanDir = "/var/lib/siroc/scans"

func (m *Manager) Scanners() *rpc.ScannersStatus {
	SyncLocalVhostHosts()
	st := &rpc.ScannersStatus{
		Tools: []rpc.ScannerTool{
			scannerTool("nikto", "Nikto", "nikto"),
			scannerTool("zap", "OWASP ZAP", zapBin()),
			scannerTool("openvas", "OpenVAS / Greenbone", openvasBin()),
		},
	}
	return st
}

func scannerTool(id, title, bin string) rpc.ScannerTool {
	t := rpc.ScannerTool{ID: id, Title: title}
	if bin == "" {
		return t
	}
	if _, err := os.Stat(bin); err == nil {
		t.Installed = true
		t.Binary = bin
		return t
	}
	if p, err := exec.LookPath(filepath.Base(bin)); err == nil {
		t.Installed = true
		t.Binary = p
	} else if p, err := exec.LookPath(bin); err == nil {
		t.Installed = true
		t.Binary = p
	}
	return t
}

func zapBin() string {
	if _, err := os.Stat("/opt/zaproxy/zap.sh"); err == nil {
		return "/opt/zaproxy/zap.sh"
	}
	if p, err := exec.LookPath("zap.sh"); err == nil {
		return p
	}
	return "zap.sh"
}

func openvasBin() string {
	if p, err := exec.LookPath("gvm-cli"); err == nil {
		return p
	}
	if p, err := exec.LookPath("gvmd"); err == nil {
		return p
	}
	return "gvm-cli"
}

func (m *Manager) RunScan(req rpc.ScanReq) (*rpc.ScanResp, error) {
	target, err := validate.ScanTarget(req.Target)
	if err != nil {
		return nil, err
	}
	tool := strings.ToLower(strings.TrimSpace(req.Tool))
	if err := os.MkdirAll(scanDir, 0750); err != nil {
		return nil, err
	}
	id := time.Now().UTC().Format("20060102-150405") + "-" + tool
	pinScanHost(target)
	switch tool {
	case "nikto":
		return runNikto(id, target)
	case "zap":
		return runZAP(id, target)
	case "openvas":
		return runOpenVAS(id, target)
	default:
		return nil, fmt.Errorf("unknown scanner %q", req.Tool)
	}
}

func runNikto(id, target string) (*rpc.ScanResp, error) {
	if _, err := exec.LookPath("nikto"); err != nil {
		return nil, fmt.Errorf("Nikto is not installed; install it from Software")
	}
	outFile := filepath.Join(scanDir, id+".txt")
	cmd := exec.Command("nikto", "-h", target, "-output", outFile, "-Format", "txt")
	raw, err := combinedScan(cmd, 10*time.Minute)
	body, _ := os.ReadFile(outFile)
	summary := strings.TrimSpace(string(body))
	if summary == "" {
		summary = strings.TrimSpace(raw)
	}
	if err != nil && summary == "" {
		return nil, fmt.Errorf("nikto: %s", tailScan(raw))
	}
	resp := &rpc.ScanResp{OK: err == nil, ID: id, Tool: "nikto", Target: target, Report: "/api/security/scans/" + id, Output: clipScan(summary)}
	enrichScan(resp, summary)
	return resp, nil
}

func runZAP(id, target string) (*rpc.ScanResp, error) {
	bin := zapBin()
	if _, err := os.Stat(bin); err != nil {
		if p, look := exec.LookPath("zap.sh"); look == nil {
			bin = p
		} else {
			return nil, fmt.Errorf("OWASP ZAP is not installed; install it from Software")
		}
	}
	port, err := freeLocalPort()
	if err != nil {
		port = 18080
	}
	home := zapHomeDir
	_ = os.MkdirAll(home, 0750)
	ensureZAPAddons()
	outFile := filepath.Join(scanDir, id+".html")
	portS := strconv.Itoa(port)
	cmd := exec.Command(bin,
		"-cmd",
		"-silent",
		"-notel",
		"-host", "127.0.0.1",
		"-port", portS,
		"-dir", home,
		"-config", "start.dayLastChecked="+zapCheckedToday(),
		"-config", "network.localServers.mainProxy.address=127.0.0.1",
		"-config", "network.localServers.mainProxy.port="+portS,
		"-quickurl", target,
		"-quickout", outFile,
		"-quickprogress",
	)
	cmd.Env = append(os.Environ(), "HOME="+home, "ZAP_PORT="+portS)
	raw, err := combinedScan(cmd, 12*time.Minute)
	body, _ := os.ReadFile(outFile)
	log := zapCleanLog(raw)
	if err != nil && len(body) == 0 {
		msg := zapFriendlyErr(raw, err)
		return nil, fmt.Errorf("zap: %s", msg)
	}
	ok := err == nil && !zapAttackFailed(raw)
	if zapAttackFailed(raw) {
		ok = false
	}
	resp := &rpc.ScanResp{OK: ok, ID: id, Tool: "zap", Target: target, Report: "/api/security/scans/" + id, Output: clipScan(log)}
	enrichScan(resp, string(body)+"\n"+log)
	return resp, nil
}

func zapAttackFailed(raw string) bool {
	low := strings.ToLower(raw)
	return strings.Contains(low, "failed to attack the url") ||
		strings.Contains(low, "name or service not known") ||
		strings.Contains(low, "unknown host")
}

func zapCleanLog(raw string) string {
	var keep []string
	for _, line := range strings.Split(raw, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		low := strings.ToLower(t)
		switch {
		case strings.HasPrefix(low, "found java version"):
			continue
		case strings.HasPrefix(low, "available memory"):
			continue
		case strings.HasPrefix(low, "using jvm args"):
			continue
		case strings.Contains(low, "no check for updates"):
			continue
		case strings.Contains(low, "add-ons may well be out of date"):
			continue
		case strings.HasPrefix(low, "zap report saved"):
			continue
		}
		keep = append(keep, t)
	}
	return strings.Join(keep, "\n")
}

func zapFriendlyErr(raw string, err error) string {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	low := strings.ToLower(raw + " " + msg)
	if strings.Contains(low, "address already in use") || strings.Contains(low, "failed to start the main proxy") {
		return "ZAP could not bind its local proxy. Apache already uses port 8080 on this server; retry the scan (it now uses a free port)."
	}
	if zapAttackFailed(raw) {
		return zapFailSummary(raw)
	}
	return tailScan(raw)
}

func zapFailSummary(raw string) string {
	low := strings.ToLower(raw)
	if strings.Contains(low, "name or service not known") || strings.Contains(low, "unknown host") {
		return "ZAP could not resolve the site name. Hosted domains are now written to /etc/hosts — run the scan again."
	}
	if i := strings.Index(low, "failed to attack the url"); i >= 0 {
		line := raw
		for _, l := range strings.Split(raw, "\n") {
			if strings.Contains(strings.ToLower(l), "failed to attack the url") {
				line = strings.TrimSpace(l)
				break
			}
		}
		return "ZAP could not open the site: " + line
	}
	return "ZAP could not attack the URL. Check that the site is up and the hostname resolves on this server."
}

func freeLocalPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func runOpenVAS(id, target string) (*rpc.ScanResp, error) {
	if _, err := exec.LookPath("gvm-cli"); err != nil {
		if _, err2 := exec.LookPath("gvmd"); err2 != nil {
			return nil, fmt.Errorf("OpenVAS/GVM is not installed; install it from Software")
		}
	}
	if err := PrepareOpenVAS(); err != nil {
		return nil, err
	}
	host := targetHost(target)
	pass := openvasPassword()
	sock := gvmSocket()
	if sock == "" {
		return nil, fmt.Errorf("gvmd socket not found after start: %s", gvmLogTail())
	}
	ver, err := gvmXML(sock, pass, "<get_version/>")
	if err != nil {
		return nil, fmt.Errorf("OpenVAS is not ready (%s). Feeds may still be syncing", err)
	}
	name := "siroc-" + id
	created, err := gvmXML(sock, pass, fmt.Sprintf(`<create_target><name>%s</name><hosts>%s</hosts><port_range>T:80,443,8080,8443,8444</port_range></create_target>`, xmlEscape(name), xmlEscape(host)))
	if err != nil {
		return nil, fmt.Errorf("create target: %s", err)
	}
	tid := xmlID(created)
	if tid == "" {
		return nil, fmt.Errorf("create target: %s", clipScan(created))
	}
	cfg, _ := gvmXML(sock, pass, `<get_configs/>`)
	cid := xmlNamedID(cfg, "Full and fast")
	if cid == "" {
		cid = xmlNamedID(cfg, "Full and fast ultimate")
	}
	if cid == "" {
		cid = xmlNamedID(cfg, "Discovery")
	}
	if cid == "" {
		_ = exec.Command("runuser", "-u", "_gvm", "--", "gvmd", "--rebuild-gvmd-data=configs,port_lists").Run()
		cfg, _ = gvmXML(sock, pass, `<get_configs/>`)
		cid = xmlNamedID(cfg, "Full and fast")
	}
	if cid == "" {
		return nil, fmt.Errorf("OpenVAS has no scan configs yet. Vulnerability feeds are still downloading — wait a few minutes and retry")
	}
	scanners, _ := gvmXML(sock, pass, `<get_scanners/>`)
	sid := xmlNamedID(scanners, "OpenVAS Default")
	if sid == "" {
		sid = xmlNamedID(scanners, "OpenVAS")
	}
	if sid == "" {
		sid = firstUUID(scanners)
	}
	taskXML := fmt.Sprintf(`<create_task><name>%s</name><target id="%s"/><config id="%s"/>`, xmlEscape(name), tid, cid)
	if sid != "" {
		taskXML += fmt.Sprintf(`<scanner id="%s"/>`, sid)
	}
	taskXML += `</create_task>`
	taskOut, err := gvmXML(sock, pass, taskXML)
	if err != nil {
		return nil, fmt.Errorf("create task: %s", err)
	}
	taskID := xmlID(taskOut)
	if taskID == "" {
		return nil, fmt.Errorf("create task: %s", clipScan(taskOut))
	}
	start, err := gvmXML(sock, pass, fmt.Sprintf(`<start_task task_id="%s"/>`, taskID))
	if err != nil {
		return nil, fmt.Errorf("start task: %s", err)
	}
	summary := "OpenVAS task started.\n" + clipScan(ver) + "\n" + clipScan(start)
	if pluginCount() < 50 {
		summary += "\nNote: the Greenbone NVT feed is still downloading, so this run may find few or no vulnerabilities until sync finishes (often 20–40 minutes)."
	}
	outFile := filepath.Join(scanDir, id+".txt")
	_ = os.WriteFile(outFile, []byte(summary+"\nGSA UI is usually https://<host>:9392\n"), 0640)
	resp := &rpc.ScanResp{OK: true, ID: id, Tool: "openvas", Target: target, Report: "/api/security/scans/" + id, Output: clipScan(summary)}
	enrichScan(resp, summary)
	return resp, nil
}

func (m *Manager) ScanReport(id string) (string, []byte, error) {
	if !scanIDOK(id) {
		return "", nil, fmt.Errorf("invalid scan id")
	}
	for _, ext := range []string{".html", ".txt", ".xml"} {
		p := filepath.Join(scanDir, id+ext)
		b, err := os.ReadFile(p)
		if err == nil {
			ctype := "text/plain; charset=utf-8"
			if ext == ".html" {
				ctype = "text/html; charset=utf-8"
			}
			return ctype, b, nil
		}
	}
	return "", nil, fmt.Errorf("report not found")
}

func gvmXML(sock, pass, xml string) (string, error) {
	cmd := exec.Command("runuser", "-u", "_gvm", "--", "gvm-cli", "--gmp-username", "admin", "--gmp-password", pass, "socket", "--socketpath", sock, "--xml", xml)
	out, err := combinedScan(cmd, 45*time.Second)
	if err != nil {
		return out, fmt.Errorf("%s", tailScan(out))
	}
	return out, nil
}

func gvmSocket() string {
	for _, p := range []string{"/run/gvmd/gvmd.sock", "/var/run/gvmd/gvmd.sock", "/tmp/gvm/gvmd/gvmd.sock"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func openvasPassword() string {
	b, err := os.ReadFile("/var/lib/siroc/openvas.pass")
	if err == nil && strings.TrimSpace(string(b)) != "" {
		return strings.TrimSpace(string(b))
	}
	return "admin"
}

func targetHost(target string) string {
	s := strings.TrimPrefix(strings.TrimPrefix(target, "https://"), "http://")
	s = strings.TrimSuffix(s, "/")
	if i := strings.IndexAny(s, "/:"); i >= 0 {
		s = s[:i]
	}
	return s
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}

var uuidRe = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

func xmlID(s string) string {
	return uuidRe.FindString(s)
}

func xmlNamedID(s, name string) string {
	low := strings.ToLower(s)
	idx := strings.Index(low, strings.ToLower(name))
	if idx < 0 {
		return ""
	}
	window := s[:idx]
	if len(window) > 400 {
		window = window[len(window)-400:]
	}
	ids := uuidRe.FindAllString(window, -1)
	if len(ids) == 0 {
		return ""
	}
	return ids[len(ids)-1]
}

func firstUUID(s string) string {
	return uuidRe.FindString(s)
}

func scanIDOK(id string) bool {
	ok, _ := regexp.MatchString(`^[0-9]{8}-[0-9]{6}-[a-z0-9-]+$`, id)
	return ok && !strings.Contains(id, "..")
}

func combinedScan(cmd *exec.Cmd, d time.Duration) (string, error) {
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

func clipScan(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 12000 {
		return s[len(s)-12000:]
	}
	return s
}

func tailScan(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 800 {
		return s[len(s)-800:]
	}
	return s
}

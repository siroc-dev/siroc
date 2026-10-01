//go:build linux

package weblog

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/validate"
)

func Generate(domain string) error {
	if err := validate.Domain(domain); err != nil {
		return err
	}
	ensureGoAccessPkg()
	if _, err := exec.LookPath("goaccess"); err != nil {
		return fmt.Errorf("GoAccess is not installed. Install it from Software.")
	}
	if err := EnsureLogDirs(); err != nil {
		return err
	}
	logs := accessLogFiles(domain)
	out := ReportPath(domain)
	if len(logs) == 0 {
		return os.WriteFile(out, []byte(emptyReport(domain)), 0644)
	}
	days := strconv.Itoa(Retention())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	help := goaccessHelp()
	dbs := EnsureGeoIP()
	skip := map[string]bool{}
	var b []byte
	var err error
	for i := 0; i < 6; i++ {
		args := goaccessReportArgs(help, logs, out, days, domain+" stats", dbs, skip)
		cmd := exec.CommandContext(ctx, "goaccess", args...)
		cmd.Env = append(os.Environ(), "LC_ALL=C")
		b, err = cmd.CombinedOutput()
		if err == nil {
			break
		}
		if !goaccessSkipFromError(string(b), skip) {
			return fmt.Errorf("goaccess: %s", strings.TrimSpace(string(b)))
		}
	}
	if err != nil {
		return fmt.Errorf("goaccess: %s", strings.TrimSpace(string(b)))
	}
	_ = os.Chmod(out, 0644)
	return nil
}

func GenerateAll() error {
	ensureGoAccessPkg()
	if _, err := exec.LookPath("goaccess"); err != nil {
		return nil
	}
	var first error
	for _, d := range SiteDomains() {
		if err := Generate(d); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func ReadReport(domain string) ([]byte, error) {
	if err := validate.Domain(domain); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(ReportPath(domain))
	if err != nil {
		return nil, err
	}
	return b, nil
}

func accessLogFiles(domain string) []string {
	cur := NginxAccessLog(domain)
	matches, _ := filepath.Glob(cur + "*")
	var out []string
	seen := map[string]struct{}{}
	for _, p := range matches {
		st, err := os.Stat(p)
		if err != nil || st.IsDir() || st.Size() == 0 {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func emptyReport(domain string) string {
	return `<!doctype html><html><head><meta charset="utf-8"><title>` + domain + ` stats</title>
<style>body{font-family:system-ui,sans-serif;margin:48px;color:#1f2937}</style></head>
<body><h1>` + domain + `</h1><p>No requests in the access log yet. Traffic through nginx will appear here.</p></body></html>`
}

func goaccessHelp() string {
	out, _ := exec.Command("goaccess", "--help").CombinedOutput()
	return string(out)
}

func goaccessVersionRaw() string {
	out, _ := exec.Command("goaccess", "-V").CombinedOutput()
	if strings.TrimSpace(string(out)) == "" {
		out, _ = exec.Command("goaccess", "--version").CombinedOutput()
	}
	return string(out)
}

func tryOfficialGoAccess() {
	if _, err := exec.LookPath("goaccess"); err == nil && goaccessAtLeast(goaccessVersionRaw(), "1.9.4") {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	env := append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	_ = exec.CommandContext(ctx, "apt-get", "-y", "install", "ca-certificates", "curl", "gnupg").Run()
	key := "/usr/share/keyrings/goaccess.gpg"
	if _, err := os.Stat(key); err != nil {
		cmd := exec.CommandContext(ctx, "bash", "-lc", "curl -fsSL https://deb.goaccess.io/gnugpg.key | gpg --dearmor -o "+key)
		cmd.Env = env
		_ = cmd.Run()
	}
	codename := "noble"
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "VERSION_CODENAME=") {
				codename = strings.Trim(strings.TrimPrefix(line, "VERSION_CODENAME="), `"'`)
			}
		}
	}
	for _, suite := range []string{codename, "noble", "jammy", "bookworm"} {
		line := "deb [signed-by=" + key + " arch=amd64] https://deb.goaccess.io/ " + suite + " main\n"
		_ = os.WriteFile("/etc/apt/sources.list.d/goaccess.list", []byte(line), 0644)
		upd := exec.CommandContext(ctx, "apt-get", "update", "-qq")
		upd.Env = env
		if upd.Run() != nil {
			continue
		}
		inst := exec.CommandContext(ctx, "apt-get", "-y", "install", "goaccess", "libmaxminddb0")
		inst.Env = env
		_ = inst.Run()
		if goaccessAtLeast(goaccessVersionRaw(), "1.9.4") {
			return
		}
	}
}

func ensureGoAccessPkg() {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	if _, err := exec.LookPath("goaccess"); err != nil {
		upd := exec.CommandContext(ctx, "apt-get", "update", "-qq")
		upd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
		_ = upd.Run()
		cmd := exec.CommandContext(ctx, "apt-get", "-y", "install", "goaccess", "libmaxminddb0")
		cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
		_ = cmd.Run()
	}
	tryOfficialGoAccess()
	_ = EnsureGeoIP()
}

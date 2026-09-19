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
	args := append([]string{}, logs...)
	args = append(args,
		"--no-global-config",
		"--log-format=COMBINED",
		"-a",
		"--keep-last", days,
		"--html-report-title", domain+" stats",
		"-o", out,
	)
	cmd := exec.CommandContext(ctx, "goaccess", args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	b, err := cmd.CombinedOutput()
	if err != nil {
		// Older goaccess without --keep-last.
		if strings.Contains(string(b), "keep-last") || strings.Contains(string(b), "unrecognized") {
			args = append([]string{}, logs...)
			args = append(args, "--no-global-config", "--log-format=COMBINED", "-a", "--html-report-title", domain+" stats", "-o", out)
			cmd = exec.CommandContext(ctx, "goaccess", args...)
			cmd.Env = append(os.Environ(), "LC_ALL=C")
			b, err = cmd.CombinedOutput()
		}
		if err != nil {
			return fmt.Errorf("goaccess: %s", strings.TrimSpace(string(b)))
		}
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

func ensureGoAccessPkg() {
	if _, err := exec.LookPath("goaccess"); err == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	upd := exec.CommandContext(ctx, "apt-get", "update", "-qq")
	upd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	_ = upd.Run()
	cmd := exec.CommandContext(ctx, "apt-get", "-y", "install", "goaccess")
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	_ = cmd.Run()
}

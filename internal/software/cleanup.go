//go:build linux

package software

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func (m *Manager) CleanTemp() rpc.CleanupResult {
	return cleanTemp()
}

func (m *Manager) CleanLogs() rpc.CleanupResult {
	return cleanLogs()
}

func cleanTemp() rpc.CleanupResult {
	ensureDiskTmp()
	var files int
	var freed int64
	add := func(n int, b int64) {
		files += n
		freed += b
	}

	for _, pat := range []string{
		"/tmp/gvmd-split-xml-file-*",
		"/tmp/node-compile-cache",
		"/tmp/cp-zap",
		"/tmp/temp-*",
		"/var/tmp/gvm/gvmd-split-xml-file-*",
		"/var/lib/apt/lists/partial",
	} {
		n, b := removeGlob(pat)
		add(n, b)
	}
	for _, name := range []string{"siroc-hello.txt", "pkgs.json", "scanners-out", "ga-install.log", "gvm-settings.xml", "core-js-banners", "zapprobe.html", "sock-test.py"} {
		n, b := removePath(filepath.Join("/tmp", name))
		add(n, b)
	}
	add(emptyDir("/opt/siroc/.tmp"))
	add(emptyDir("/opt/siroc/.cache"))
	add(emptyDir("/opt/siroc/.gotmp"))
	add(emptyDir("/var/tmp/cp-apt"))

	cmd := aptCmd("clean")
	_, _ = combinedTimeout(cmd, 45*time.Second)

	return rpc.CleanupResult{
		OK:      true,
		Freed:   freed,
		Files:   files,
		Message: fmt.Sprintf("Freed %s (%d items)", humanBytes(freed), files),
	}
}

func cleanLogs() rpc.CleanupResult {
	var files int
	var freed int64
	add := func(n int, b int64) {
		files += n
		freed += b
	}

	before := journalBytes()
	vac := exec.Command("journalctl", "--vacuum-time=7d")
	vac.Env = aptEnv()
	_, _ = combinedTimeout(vac, 45*time.Second)
	if after := journalBytes(); before > after {
		freed += before - after
	}

	_ = filepath.WalkDir("/var/log", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := d.Name()
			if base == "journal" || base == "private" {
				return fs.SkipDir
			}
			return nil
		}
		if !isRotatedLog(d.Name()) {
			return nil
		}
		n, b := removePath(path)
		add(n, b)
		return nil
	})

	for _, p := range []string{"/var/log/gvm/gvmd.log", "/var/log/gvm/gsad.log", "/var/log/gvm/ospd-openvas.log"} {
		n, b := truncateIfHuge(p, 20*1024*1024)
		add(n, b)
	}

	return rpc.CleanupResult{
		OK:      true,
		Freed:   freed,
		Files:   files,
		Message: fmt.Sprintf("Freed %s of logs (%d files)", humanBytes(freed), files),
	}
}

var rotatedLogRe = regexp.MustCompile(`\.(gz|xz|bz2|zst|old|[1-9]|[1-9][0-9])$`)

func isRotatedLog(name string) bool {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".journal") || strings.HasSuffix(lower, ".journal~") {
		return false
	}
	return rotatedLogRe.MatchString(name)
}

func removeGlob(pattern string) (int, int64) {
	matches, _ := filepath.Glob(pattern)
	var files int
	var freed int64
	for _, p := range matches {
		n, b := removePath(p)
		files += n
		freed += b
	}
	return files, freed
}

func emptyDir(dir string) (int, int64) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0
	}
	var files int
	var freed int64
	for _, e := range ents {
		n, b := removePath(filepath.Join(dir, e.Name()))
		files += n
		freed += b
	}
	return files, freed
}

func removePath(path string) (int, int64) {
	st, err := os.Lstat(path)
	if err != nil {
		return 0, 0
	}
	if st.Mode()&os.ModeSocket != 0 || st.Mode()&os.ModeNamedPipe != 0 {
		return 0, 0
	}
	bytes := fileBytes(path, st)
	files := 1
	if st.IsDir() {
		_ = filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				files++
			}
			return nil
		})
	}
	if err := os.RemoveAll(path); err != nil {
		return 0, 0
	}
	return files, bytes
}

func fileBytes(path string, st os.FileInfo) int64 {
	if !st.IsDir() {
		return st.Size()
	}
	var total int64
	_ = filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

func truncateIfHuge(path string, max int64) (int, int64) {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() || st.Size() <= max {
		return 0, 0
	}
	freed := st.Size()
	if err := os.Truncate(path, 0); err != nil {
		return 0, 0
	}
	return 1, freed
}

func journalBytes() int64 {
	out, err := exec.Command("journalctl", "--disk-usage").CombinedOutput()
	if err != nil {
		return 0
	}
	return parseJournalBytes(string(out))
}

func parseJournalBytes(s string) int64 {
	fields := strings.Fields(s)
	for i, f := range fields {
		if i+1 >= len(fields) {
			continue
		}
		n, ok := parseSizeToken(f, fields[i+1])
		if ok {
			return n
		}
	}
	return 0
}

func parseSizeToken(num, unit string) (int64, bool) {
	var v float64
	if _, err := fmt.Sscanf(num, "%f", &v); err != nil {
		return 0, false
	}
	mult := int64(1)
	switch strings.ToUpper(strings.TrimRight(unit, ".,;")) {
	case "B":
		mult = 1
	case "K", "KB", "KIB":
		mult = 1024
	case "M", "MB", "MIB":
		mult = 1024 * 1024
	case "G", "GB", "GIB":
		mult = 1024 * 1024 * 1024
	default:
		return 0, false
	}
	return int64(v * float64(mult)), true
}

func humanBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	u := []string{"B", "KB", "MB", "GB", "TB"}
	v := float64(n)
	i := 0
	for v >= 1024 && i < len(u)-1 {
		v /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", n)
	}
	if v >= 10 {
		return fmt.Sprintf("%.0f %s", v, u[i])
	}
	return fmt.Sprintf("%.1f %s", v, u[i])
}

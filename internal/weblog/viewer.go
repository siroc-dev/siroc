//go:build linux

package weblog

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/validate"
)

func SiteLogs(req rpc.SiteLogReq) (*rpc.SiteLogsResp, error) {
	if err := validate.LinuxUser(req.Username); err != nil {
		return nil, err
	}
	if err := validate.Domain(req.Domain); err != nil {
		return nil, err
	}
	_ = TouchSiteLogs(req.Domain)
	files, err := listSiteLogFiles(req.Username, req.Domain, req.DocRoot)
	if err != nil {
		return nil, err
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		id = pickDefaultID(files)
	}
	group, name, ok := ParseSiteLogID(id)
	if !ok {
		return nil, fmt.Errorf("unknown log %q", req.ID)
	}
	out := &rpc.SiteLogsResp{OK: true, Domain: req.Domain, Files: files, Entries: []rpc.SiteLogEntry{}}
	want := id
	if group == "laravel" && name == "laravel.log" && (id == "laravel" || id == "") {
		want = pickLaravelID(files)
	}
	var current *rpc.SiteLogFile
	for i := range files {
		if files[i].ID == want || (id == "laravel" && files[i].ID == "laravel") {
			current = &files[i]
			break
		}
	}
	if current == nil {
		current = &rpc.SiteLogFile{ID: id, Label: labelFor(group, name), Group: group, Kind: kindFor(group, name), Path: resolveSiteLogPath(req, group, name)}
	}
	out.Current = current
	if current.Path == "" {
		return out, nil
	}
	content, size, trunc, err := ReadTail(current.Path, req.Bytes)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if os.IsNotExist(err) {
		current.Exists = false
		if current.Group == "waf" {
			if g, _, gTrunc, gerr := ReadTail(GlobalWAFLog(), req.Bytes); gerr == nil {
				if filtered := FilterWAFByHost(g, req.Domain); strings.TrimSpace(filtered) != "" {
					out.Content = filtered
					out.Truncated = gTrunc
					out.Entries = ParseLogEntries("waf", "audit", filtered)
					out.Counts = CountLevels(out.Entries)
				}
			}
		}
		return out, nil
	}
	current.Exists = true
	current.Size = size
	out.Content = content
	out.Truncated = trunc
	if current.Group == "waf" {
		filtered := FilterWAFByHost(content, req.Domain)
		if strings.TrimSpace(filtered) == "" {
			if g, _, gTrunc, gerr := ReadTail(GlobalWAFLog(), req.Bytes); gerr == nil {
				filtered = FilterWAFByHost(g, req.Domain)
				out.Truncated = gTrunc
			}
		}
		content = filtered
		out.Content = filtered
	}
	out.Entries = ParseLogEntries(current.Group, current.Kind, content)
	out.Counts = CountLevels(out.Entries)
	return out, nil
}

func pickDefaultID(files []rpc.SiteLogFile) string {
	if id := pickLaravelID(files); id != "laravel" {
		return id
	}
	for _, f := range files {
		if f.ID == "laravel" && f.Exists {
			return "laravel"
		}
	}
	return "nginx-access"
}

func listSiteLogFiles(user, domain, doc string) ([]rpc.SiteLogFile, error) {
	files := []rpc.SiteLogFile{
		siteLogMeta("nginx-access", "Nginx access", "nginx", "access", NginxAccessLog(domain)),
		siteLogMeta("nginx-error", "Nginx error", "nginx", "error", NginxErrorLog(domain)),
		siteLogMeta("apache-access", "Apache access", "apache", "access", ApacheAccessLog(domain)),
		siteLogMeta("apache-error", "Apache error", "apache", "error", ApacheErrorLog(domain)),
		siteLogMeta("waf-audit", "ModSecurity WAF", "waf", "audit", ApacheWAFLog(domain)),
	}
	dir := laravelLogDir(user, doc)
	if dir == "" {
		return files, nil
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		files = append(files, rpc.SiteLogFile{ID: "laravel", Label: "Laravel", Group: "laravel", Kind: "app", Path: filepath.Join(dir, "laravel.log")})
		return files, nil
	}
	hasAny := false
	for _, e := range ents {
		if e.IsDir() || !ValidLaravelLogName(e.Name()) {
			continue
		}
		id := "laravel:" + e.Name()
		label := "Laravel · " + e.Name()
		if e.Name() == "laravel.log" {
			id = "laravel"
			label = "Laravel"
		}
		hasAny = true
		files = append(files, siteLogMeta(id, label, "laravel", "app", filepath.Join(dir, e.Name())))
	}
	if !hasAny {
		files = append(files, rpc.SiteLogFile{ID: "laravel", Label: "Laravel", Group: "laravel", Kind: "app", Path: filepath.Join(dir, "laravel.log")})
	}
	return files, nil
}

func siteLogMeta(id, label, group, kind, path string) rpc.SiteLogFile {
	f := rpc.SiteLogFile{ID: id, Label: label, Group: group, Kind: kind, Path: path}
	st, err := os.Stat(path)
	if err != nil {
		return f
	}
	f.Exists = true
	f.Size = st.Size()
	f.ModTime = st.ModTime().UTC().Format(time.RFC3339)
	return f
}

func resolveSiteLogPath(req rpc.SiteLogReq, group, name string) string {
	switch group {
	case "nginx":
		if name == "error" {
			return NginxErrorLog(req.Domain)
		}
		return NginxAccessLog(req.Domain)
	case "apache":
		if name == "error" {
			return ApacheErrorLog(req.Domain)
		}
		return ApacheAccessLog(req.Domain)
	case "waf":
		return ApacheWAFLog(req.Domain)
	case "laravel":
		dir := laravelLogDir(req.Username, req.DocRoot)
		if dir == "" {
			return ""
		}
		return filepath.Join(dir, name)
	}
	return ""
}

func laravelLogDir(user, docRoot string) string {
	doc := filepath.Clean(docRoot)
	if doc == "" || doc == "." {
		return ""
	}
	home := filepath.Join("/home", user)
	if !InHome(home, doc) {
		return ""
	}
	cur := doc
	for i := 0; i < 8; i++ {
		if !InHome(home, cur) {
			break
		}
		if dir := laravelLogsIfApp(home, cur); dir != "" {
			return dir
		}
		if i == 0 {
			if ents, err := os.ReadDir(cur); err == nil {
				for _, e := range ents {
					if !e.IsDir() {
						continue
					}
					if dir := laravelLogsIfApp(home, filepath.Join(cur, e.Name())); dir != "" {
						return dir
					}
				}
			}
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return ""
}

func laravelLogsIfApp(home, root string) string {
	if !InHome(home, root) {
		return ""
	}
	if _, err := os.Stat(filepath.Join(root, "artisan")); err != nil {
		return ""
	}
	dir := filepath.Join(root, "storage", "logs")
	if !InHome(home, dir) {
		return ""
	}
	return dir
}

func pickLaravelID(files []rpc.SiteLogFile) string {
	var newest string
	var latest time.Time
	for _, f := range files {
		if f.Group != "laravel" {
			continue
		}
		if f.ID == "laravel" && f.Exists {
			return "laravel"
		}
		if !f.Exists || f.ModTime == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, f.ModTime)
		if err != nil || t.Before(latest) {
			continue
		}
		latest = t
		newest = f.ID
	}
	if newest != "" {
		return newest
	}
	return "laravel"
}

func labelFor(group, name string) string {
	switch group {
	case "nginx":
		if name == "error" {
			return "Nginx error"
		}
		return "Nginx access"
	case "apache":
		if name == "error" {
			return "Apache error"
		}
		return "Apache access"
	case "waf":
		return "ModSecurity WAF"
	default:
		if name == "laravel.log" || name == "" {
			return "Laravel"
		}
		return "Laravel · " + name
	}
}

func kindFor(group, name string) string {
	if group == "laravel" {
		return "app"
	}
	if group == "waf" {
		return "audit"
	}
	if name == "error" {
		return "error"
	}
	return "access"
}

func ReadTail(path string, max int) (string, int64, bool, error) {
	max = ClampTailBytes(max)
	f, err := os.Open(path)
	if err != nil {
		return "", 0, false, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", 0, false, err
	}
	size := st.Size()
	if size <= 0 {
		return "", 0, false, nil
	}
	start := size - int64(max)
	if start < 0 {
		start = 0
	}
	if start > 0 {
		if _, err := f.Seek(start, io.SeekStart); err != nil {
			return "", size, false, err
		}
	}
	b, err := io.ReadAll(io.LimitReader(f, int64(max)+4096))
	if err != nil {
		return "", size, false, err
	}
	out, trunc := TailBytes(b, max)
	if start == 0 {
		trunc = false
	}
	return string(out), size, trunc, nil
}

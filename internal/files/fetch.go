//go:build linux

package files

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func (m *Manager) Fetch(username, dir, rawURL, dest string, root bool) (*rpc.FileOpResp, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("url must be http or https")
	}
	name := dest
	if name == "" {
		name = filepath.Base(u.Path)
	}
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == "/" {
		name = "download"
	}
	target := strings.TrimSuffix(dir, "/") + "/" + name
	if root {
		absDir, err := resolveRoot(dir)
		if err != nil {
			return nil, err
		}
		abs := filepath.Join(absDir, name)
		if err := curlFetch(abs, u.String()); err != nil {
			return nil, err
		}
		return &rpc.FileOpResp{OK: true, Dest: abs, Path: abs}, nil
	}
	abs, home, uid, gid, err := m.resolve(username, target)
	if err != nil {
		return nil, err
	}
	resp, err := m.call(uid, gid, helperReq{Op: "fetch", Path: abs, Home: home, URL: u.String()})
	if err != nil {
		return nil, err
	}
	show := target
	if resp != nil && resp.Dest != "" {
		if rel, e := filepath.Rel(home, resp.Dest); e == nil {
			show = "/" + filepath.ToSlash(rel)
		}
	}
	return &rpc.FileOpResp{OK: true, Dest: show, Path: show}, nil
}

func (m *Manager) Search(username, dir, query string, root bool) (*rpc.FileSearchResp, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, fmt.Errorf("search query required")
	}
	if len(q) > 200 {
		return nil, fmt.Errorf("query too long")
	}
	if root {
		abs, err := resolveRoot(dir)
		if err != nil {
			return nil, err
		}
		hits, trunc := runSearch(abs, "/", q)
		return &rpc.FileSearchResp{Hits: hits, Truncated: trunc}, nil
	}
	abs, home, uid, gid, err := m.resolve(username, dir)
	if err != nil {
		return nil, err
	}
	resp, err := m.call(uid, gid, helperReq{Op: "search", Path: abs, Home: home, Query: q})
	if err != nil {
		return nil, err
	}
	hits := []rpc.FileSearchHit{}
	if resp != nil && resp.Hits != nil {
		hits = resp.Hits
	}
	trunc := resp != nil && resp.Truncated
	return &rpc.FileSearchResp{Hits: hits, Truncated: trunc}, nil
}

func curlFetch(dest, rawURL string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0750); err != nil {
		return err
	}
	cmd := exec.Command("curl", "-fL", "--retry", "2", "--max-filesize", "536870912", "--max-time", "300", "-o", dest, rawURL)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("download: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func runSearch(root, home, query string) ([]rpc.FileSearchHit, bool) {
	bin := "grep"
	args := []string{"-R", "-n", "-I", "-m", "3", "--exclude-dir=.git", "--exclude-dir=node_modules", "--exclude-dir=vendor", "--exclude-dir=.svn", query, root}
	if _, err := exec.LookPath("rg"); err == nil {
		bin = "rg"
		args = []string{"-n", "--hidden", "--no-heading", "-m", "3", "-g", "!.git", "-g", "!node_modules", "-g", "!vendor", query, root}
	}
	cmd := exec.Command(bin, args...)
	out, _ := cmd.CombinedOutput()
	var hits []rpc.FileSearchHit
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		path, ln, text := parseSearchLine(line)
		if path == "" {
			continue
		}
		rel := path
		if home != "" && home != "/" {
			if r, err := filepath.Rel(home, path); err == nil {
				rel = "/" + filepath.ToSlash(r)
			}
		}
		hits = append(hits, rpc.FileSearchHit{Path: rel, Line: ln, Text: text})
		if len(hits) >= 200 {
			return hits, true
		}
	}
	if hits == nil {
		hits = []rpc.FileSearchHit{}
	}
	return hits, false
}

func parseSearchLine(line string) (path string, ln int, text string) {
	// path:line:text — path may contain colons on Windows but we are Linux
	i := strings.Index(line, ":")
	if i < 0 {
		return "", 0, ""
	}
	path = line[:i]
	rest := line[i+1:]
	j := strings.Index(rest, ":")
	if j < 0 {
		return path, 0, rest
	}
	ln, _ = strconv.Atoi(rest[:j])
	text = strings.TrimSpace(rest[j+1:])
	if len(text) > 240 {
		text = text[:240]
	}
	return path, ln, text
}

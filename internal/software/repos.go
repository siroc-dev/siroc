//go:build linux

package software

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func repoReleaseProbe(mirror, suite string) repoProbe {
	client := &http.Client{Timeout: 8 * time.Second}
	saw404 := false
	for _, u := range releaseURLs(mirror, suite) {
		req, err := http.NewRequest(http.MethodGet, u, nil)
		if err != nil {
			continue
		}
		res, err := client.Do(req)
		if err != nil {
			continue
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64))
		_ = res.Body.Close()
		if res.StatusCode == http.StatusOK {
			return repoProbeOK
		}
		if res.StatusCode == http.StatusNotFound {
			saw404 = true
		}
	}
	if saw404 {
		return repoProbeMissing
	}
	return repoProbeUnknown
}

func firstWorkingSuite(mirror, id, code string) string {
	suite, disable := pickRepoSuite(mirror, id, code, repoReleaseProbe)
	if disable {
		return ""
	}
	return suite
}

func writeVendorRepo(name, keyURL, mirror, id, code, component string) error {
	suite := firstWorkingSuite(mirror, id, code)
	if suite == "" {
		removeRepo(name)
		return fmt.Errorf("%s has no apt repo for %s %s", name, id, code)
	}
	if err := writeKeyring(keyURL, "/etc/apt/keyrings/cp-"+name+".gpg"); err != nil {
		return err
	}
	line := fmt.Sprintf("deb [signed-by=/etc/apt/keyrings/cp-%s.gpg] %s %s %s", name, mirror, suite, component)
	return writeRepo(name, line)
}

func repairSirocRepos() {
	files, err := filepath.Glob("/etc/apt/sources.list.d/*.list")
	if err != nil {
		return
	}
	id, code := osRelease()
	for _, f := range files {
		if !managedRepoList(filepath.Base(f)) {
			continue
		}
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		changed := false
		var out []string
		for _, line := range strings.Split(string(raw), "\n") {
			_, url, suite, _, ok := parseDebLine(line)
			if !ok {
				out = append(out, line)
				continue
			}
			alt, disable := pickRepoSuite(url, id, code, repoReleaseProbe)
			if disable {
				out = append(out, "# disabled by siroc: no Release file for this suite")
				out = append(out, "# "+strings.TrimSpace(line))
				changed = true
				continue
			}
			if alt != "" && alt != suite {
				out = append(out, replaceDebSuite(line, alt))
				changed = true
				continue
			}
			out = append(out, line)
		}
		if !changed {
			continue
		}
		text := strings.Join(out, "\n")
		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		_ = os.WriteFile(f, []byte(text), 0644)
	}
}

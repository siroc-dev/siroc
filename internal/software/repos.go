//go:build linux

package software

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	suryKeyDeb    = "https://packages.sury.org/debsuryorg-archive-keyring.deb"
	suryKeyring   = "/usr/share/keyrings/debsuryorg-archive-keyring.gpg"
	suryKeyURL    = "https://packages.sury.org/php/apt.gpg"
	suryKeyringCP = "/etc/apt/keyrings/cp-sury-php.gpg"
)

func installSuryKeyring() (string, error) {
	if _, err := os.Stat(suryKeyring); err == nil {
		return suryKeyring, nil
	}
	_ = aptInstall("ca-certificates", "curl", "gnupg")
	tmp := filepath.Join(ensureDiskTmp(), "debsuryorg-archive-keyring.deb")
	dl := exec.Command("curl", "-fsSL", "-o", tmp, suryKeyDeb)
	if _, err := combinedTimeout(dl, 45*time.Second); err == nil {
		_ = exec.Command("dpkg", "-i", tmp).Run()
		if _, err := os.Stat(suryKeyring); err == nil {
			return suryKeyring, nil
		}
	}
	if err := writeKeyring(suryKeyURL, suryKeyringCP); err != nil {
		return "", fmt.Errorf("sury keyring: %w", err)
	}
	return suryKeyringCP, nil
}

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
	id, code := osRelease()
	files, err := filepath.Glob("/etc/apt/sources.list.d/*")
	if err != nil {
		return
	}
	for _, f := range files {
		switch strings.ToLower(filepath.Ext(f)) {
		case ".list":
			repairListFile(f, id, code)
		case ".sources":
			repairSourcesFile(f, id, code)
		}
	}
}

func repairListFile(path, id, code string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	changed := false
	var out []string
	for _, line := range strings.Split(string(raw), "\n") {
		_, url, suite, _, ok := parseDebLine(line)
		if !ok || officialArchive(url) {
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
		return
	}
	writeRepaired(path, strings.Join(out, "\n"))
}

func repairSourcesFile(path, id, code string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	blocks := splitDeb822(text)
	changed := false
	for i, block := range blocks {
		url := firstField(deb822Field(block, "URIs"))
		suite := firstField(deb822Field(block, "Suites"))
		if url == "" || suite == "" || officialArchive(url) {
			continue
		}
		if strings.EqualFold(deb822Field(block, "Enabled"), "no") {
			continue
		}
		alt, disable := pickRepoSuite(url, id, code, repoReleaseProbe)
		if disable {
			blocks[i] = replaceDeb822Field(block, "Enabled", "no")
			changed = true
			continue
		}
		if alt != "" && alt != suite {
			blocks[i] = replaceDeb822Field(block, "Suites", alt)
			changed = true
		}
	}
	if !changed {
		return
	}
	writeRepaired(path, joinDeb822(blocks))
}

func writeRepaired(path, text string) {
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	_ = os.WriteFile(path, []byte(text), 0644)
}

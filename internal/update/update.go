//go:build linux

package update

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/version"
)

const (
	workDir    = "/var/lib/siroc/updates"
	cacheFile  = "/var/lib/siroc/updates/latest.json"
	versionOut = "/opt/siroc/VERSION"
	versionAlt = "/var/lib/siroc/version"
	installRoot = "/opt/siroc"
)

type latestMeta struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256,omitempty"`
	Checked string `json:"checked,omitempty"`
}

func Status(channel string) *rpc.PanelUpdateStatus {
	st := &rpc.PanelUpdateStatus{
		OK:      true,
		Version: version.Current(),
		Channel: strings.TrimRight(strings.TrimSpace(channel), "/"),
	}
	if b, err := os.ReadFile(cacheFile); err == nil {
		var meta latestMeta
		if json.Unmarshal(b, &meta) == nil && meta.Version != "" {
			st.Latest = meta.Version
			st.PackageURL = meta.URL
			st.CheckedAt = meta.Checked
			st.Available = version.Compare(st.Version, meta.Version) < 0
		}
	}
	return st
}

func Check(channel string) (*rpc.PanelUpdateStatus, error) {
	channel = strings.TrimRight(strings.TrimSpace(channel), "/")
	if channel == "" {
		st := Status("")
		st.Message = "No update channel. Set a URL to latest.json, or apply a local package tarball."
		return st, nil
	}
	metaURL := channel
	if !strings.HasSuffix(strings.ToLower(channel), ".json") && !strings.HasSuffix(strings.ToLower(channel), ".tar.gz") && !strings.HasSuffix(strings.ToLower(channel), ".tgz") {
		metaURL = channel + "/latest.json"
	}
	if strings.HasSuffix(strings.ToLower(channel), ".tar.gz") || strings.HasSuffix(strings.ToLower(channel), ".tgz") {
		meta := latestMeta{URL: channel, Version: "", Checked: time.Now().UTC().Format(time.RFC3339)}
		st := Status(channel)
		st.PackageURL = channel
		st.Message = "Channel is a package URL. Apply to install it."
		st.CheckedAt = meta.Checked
		_ = writeCache(meta)
		return st, nil
	}
	body, err := httpGet(metaURL, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("check updates: %w", err)
	}
	var meta latestMeta
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, fmt.Errorf("invalid latest.json: %w", err)
	}
	meta.Version = strings.TrimSpace(meta.Version)
	meta.URL = strings.TrimSpace(meta.URL)
	if meta.Version == "" {
		return nil, fmt.Errorf("latest.json missing version")
	}
	if meta.URL == "" && !strings.HasSuffix(strings.ToLower(channel), ".json") {
		meta.URL = channel + "/siroc-linux-amd64.tar.gz"
	}
	if meta.URL == "" {
		return nil, fmt.Errorf("latest.json missing package url")
	}
	meta.Checked = time.Now().UTC().Format(time.RFC3339)
	if err := writeCache(meta); err != nil {
		return nil, err
	}
	st := Status(channel)
	st.Latest = meta.Version
	st.PackageURL = meta.URL
	st.CheckedAt = meta.Checked
	st.Available = version.Compare(st.Version, meta.Version) < 0
	if st.Available {
		st.Message = "Update " + meta.Version + " is available"
	} else {
		st.Message = "Siroc is up to date"
	}
	return st, nil
}

func Apply(channel, srcURL, srcPath string) (*rpc.PanelUpdateStatus, error) {
	if err := os.MkdirAll(workDir, 0750); err != nil {
		return nil, err
	}
	srcURL = strings.TrimSpace(srcURL)
	srcPath = strings.TrimSpace(srcPath)
	if srcURL == "" && srcPath == "" {
		if b, err := os.ReadFile(cacheFile); err == nil {
			var meta latestMeta
			if json.Unmarshal(b, &meta) == nil {
				srcURL = strings.TrimSpace(meta.URL)
			}
		}
	}
	var archive string
	var expectSHA string
	if srcPath != "" {
		p, err := safePath(srcPath)
		if err != nil {
			return nil, err
		}
		st, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("package not found: %w", err)
		}
		if st.IsDir() {
			if err := installFromDir(p); err != nil {
				return nil, err
			}
			scheduleRestart()
			stt := Status(channel)
			stt.Message = "Update applied. Services are restarting."
			stt.Restarting = true
			return stt, nil
		}
		archive = p
	} else if srcURL != "" {
		if !strings.HasPrefix(srcURL, "https://") && !strings.HasPrefix(srcURL, "http://") {
			return nil, fmt.Errorf("package URL must be http or https")
		}
		if b, err := os.ReadFile(cacheFile); err == nil {
			var meta latestMeta
			if json.Unmarshal(b, &meta) == nil && meta.URL == srcURL {
				expectSHA = strings.TrimSpace(meta.SHA256)
			}
		}
		dest := filepath.Join(workDir, "incoming.tar.gz")
		if err := download(srcURL, dest); err != nil {
			return nil, err
		}
		archive = dest
	} else {
		return nil, fmt.Errorf("provide a package URL or a tarball path on this server")
	}
	if expectSHA != "" {
		if err := verifySHA(archive, expectSHA); err != nil {
			return nil, err
		}
	}
	extract := filepath.Join(workDir, "extract")
	_ = os.RemoveAll(extract)
	if err := os.MkdirAll(extract, 0750); err != nil {
		return nil, err
	}
	out, err := exec.Command("tar", "-xzf", archive, "-C", extract).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("extract package: %s", strings.TrimSpace(string(out)))
	}
	root, err := findPackage(extract)
	if err != nil {
		return nil, err
	}
	if err := installFromDir(root); err != nil {
		return nil, err
	}
	scheduleRestart()
	st := Status(channel)
	st.Message = "Update applied. Services are restarting."
	st.Restarting = true
	return st, nil
}

func installFromDir(root string) error {
	agent := filepath.Join(root, "bin", "siroc-agent")
	panel := filepath.Join(root, "bin", "siroc-panel")
	index := filepath.Join(root, "web", "dist", "index.html")
	for _, p := range []string{agent, panel, index} {
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("package missing %s", strings.TrimPrefix(p, root))
		}
	}
	if err := os.MkdirAll(filepath.Join(installRoot, "bin"), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(installRoot, "web"), 0755); err != nil {
		return err
	}
	if err := copyFile(agent, "/usr/local/bin/siroc-agent", 0755); err != nil {
		return err
	}
	if err := copyFile(panel, "/usr/local/bin/siroc-panel", 0755); err != nil {
		return err
	}
	if err := copyFile(agent, filepath.Join(installRoot, "bin", "siroc-agent"), 0755); err != nil {
		return err
	}
	if err := copyFile(panel, filepath.Join(installRoot, "bin", "siroc-panel"), 0755); err != nil {
		return err
	}
	destWeb := filepath.Join(installRoot, "web", "dist")
	_ = os.RemoveAll(destWeb)
	if out, err := exec.Command("cp", "-a", filepath.Join(root, "web", "dist"), destWeb).CombinedOutput(); err != nil {
		return fmt.Errorf("copy web UI: %s", strings.TrimSpace(string(out)))
	}
	_ = exec.Command("chmod", "-R", "a+rX", destWeb).Run()
	if b, err := os.ReadFile(filepath.Join(root, "VERSION")); err == nil {
		v := strings.TrimSpace(string(b))
		if v != "" {
			_ = os.WriteFile(versionOut, []byte(v+"\n"), 0644)
			_ = os.WriteFile(versionAlt, []byte(v+"\n"), 0644)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "install.sh")); err == nil {
		_ = copyFile(filepath.Join(root, "install.sh"), filepath.Join(installRoot, "install.sh"), 0755)
	}
	return nil
}

func scheduleRestart() {
	_ = exec.Command("systemctl", "reset-failed", "siroc-update-restart.service").Run()
	cmd := exec.Command("systemd-run", "--unit=siroc-update-restart", "--on-active=2s",
		"/bin/bash", "-c", "systemctl restart siroc-agent.service; sleep 1; systemctl restart siroc-panel.service")
	_ = cmd.Start()
}

func findPackage(extract string) (string, error) {
	candidates := []string{
		extract,
		filepath.Join(extract, "siroc"),
	}
	_ = filepath.WalkDir(extract, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() || len(candidates) > 8 {
			return nil
		}
		if _, err := os.Stat(filepath.Join(path, "bin", "siroc-panel")); err == nil {
			candidates = append(candidates, path)
		}
		return nil
	})
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(c, "bin", "siroc-panel")); err == nil {
			if _, err := os.Stat(filepath.Join(c, "web", "dist", "index.html")); err == nil {
				return c, nil
			}
		}
	}
	return "", fmt.Errorf("package does not contain bin/siroc-panel and web/dist")
}

func safePath(p string) (string, error) {
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("path must be absolute")
	}
	clean := filepath.Clean(p)
	allow := []string{"/var/lib/siroc", "/opt/siroc", "/root", "/tmp", "/var/tmp"}
	ok := false
	for _, a := range allow {
		if clean == a || strings.HasPrefix(clean, a+string(os.PathSeparator)) {
			ok = true
			break
		}
	}
	if !ok {
		return "", fmt.Errorf("package path must be under /var/lib/siroc, /opt/siroc, /root, or /tmp")
	}
	return clean, nil
}

func download(rawURL, dest string) error {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Minute}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return fmt.Errorf("download failed: HTTP %d", res.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, res.Body); err != nil {
		return err
	}
	return nil
}

func httpGet(rawURL string, timeout time.Duration) ([]byte, error) {
	client := &http.Client{Timeout: timeout}
	res, err := client.Get(rawURL)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return b, nil
}

func verifySHA(path, want string) error {
	want = strings.ToLower(strings.TrimSpace(want))
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != want {
		return fmt.Errorf("package checksum mismatch")
	}
	return nil
}

func writeCache(meta latestMeta) error {
	if err := os.MkdirAll(workDir, 0750); err != nil {
		return err
	}
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cacheFile, b, 0640)
}

func copyFile(src, dest string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dest + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
}

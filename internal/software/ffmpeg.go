//go:build linux

package software

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const ffmpegRoot = "/opt/siroc-runtimes/ffmpeg"
const ffmpegVerCache = "/var/lib/siroc/ffmpeg-versions.json"

var (
	ffmpegVerMu    sync.Mutex
	ffmpegVerMem   []string
	ffmpegVerMemAt time.Time
)

func ffmpegFallbackVersions() []string {
	return []string{"8.1", "9.0"}
}

func ffmpegVersions() []string {
	ffmpegVerMu.Lock()
	defer ffmpegVerMu.Unlock()
	if len(ffmpegVerMem) > 0 && time.Since(ffmpegVerMemAt) < 6*time.Hour {
		return append([]string{}, ffmpegVerMem...)
	}
	if vers := loadFFmpegVerCache(); len(vers) > 0 {
		ffmpegVerMem = vers
		ffmpegVerMemAt = time.Now()
		if time.Since(ffmpegVerCacheMtime()) < 6*time.Hour {
			return append([]string{}, vers...)
		}
	}
	if vers := fetchFFmpegVersions(); len(vers) > 0 {
		ffmpegVerMem = vers
		ffmpegVerMemAt = time.Now()
		saveFFmpegVerCache(vers)
		return append([]string{}, vers...)
	}
	if len(ffmpegVerMem) > 0 {
		return append([]string{}, ffmpegVerMem...)
	}
	return ffmpegFallbackVersions()
}

func ffmpegDefaultVersion() string {
	vers := ffmpegVersions()
	if len(vers) == 0 {
		return "9.0"
	}
	return vers[len(vers)-1]
}

func ffmpegBin(version string) string {
	return filepath.Join(ffmpegRoot, version, "bin", "ffmpeg")
}

func ffmpegInstalledVersions() []string {
	var found []string
	ents, err := os.ReadDir(ffmpegRoot)
	if err != nil {
		return found
	}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(ffmpegBin(e.Name())); err == nil {
			found = append(found, e.Name())
		}
	}
	sort.Strings(found)
	return found
}

func currentFFmpegCLI() string {
	p, err := exec.LookPath("ffmpeg")
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	rel, err := filepath.Rel(ffmpegRoot, p)
	if err == nil && !strings.HasPrefix(rel, "..") {
		ver, _, _ := strings.Cut(rel, string(os.PathSeparator))
		return ver
	}
	out, err := exec.Command("ffmpeg", "-version").CombinedOutput()
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	line = strings.TrimPrefix(line, "ffmpeg version ")
	line = strings.TrimPrefix(line, "n")
	for _, v := range ffmpegInstalledVersions() {
		if strings.HasPrefix(line, v) {
			return v
		}
	}
	if f := strings.Fields(line); len(f) > 0 {
		return f[0]
	}
	return ""
}

func setFFmpegCLI(version string) error {
	version = strings.TrimSpace(version)
	if version == "" {
		return fmt.Errorf("missing FFmpeg version")
	}
	bin := ffmpegBin(version)
	if _, err := os.Stat(bin); err != nil {
		return fmt.Errorf("FFmpeg %s is not installed", version)
	}
	probe := filepath.Join(ffmpegRoot, version, "bin", "ffprobe")
	if err := alternativesSet("ffmpeg", bin, nil); err != nil {
		return err
	}
	if _, err := os.Stat(probe); err == nil {
		_ = alternativesSet("ffprobe", probe, nil)
	}
	return nil
}

func installFFmpeg(version string) error {
	if version == "" {
		version = ffmpegDefaultVersion()
	}
	allowed := ffmpegVersions()
	ok := false
	for _, v := range allowed {
		if v == version {
			ok = true
			break
		}
	}
	if !ok {
		return fmt.Errorf("unsupported FFmpeg version %s (available: %s)", version, strings.Join(allowed, ", "))
	}
	if _, err := os.Stat(ffmpegBin(version)); err == nil {
		return setFFmpegCLI(version)
	}
	if err := os.MkdirAll("/opt/siroc/.cache", 0755); err != nil {
		return err
	}
	archive := filepath.Join("/opt/siroc/.cache", "ffmpeg-"+version+".tar.xz")
	var last error
	for _, url := range ffmpegURLs(version) {
		_ = os.Remove(archive)
		cmd := exec.Command("curl", "-fL", "--retry", "2", "-o", archive, url)
		cmd.Env = append(os.Environ(), "TMPDIR="+ensureDiskTmp())
		if out, err := combinedTimeout(cmd, 8*time.Minute); err != nil {
			last = fmt.Errorf("download FFmpeg %s: %s", version, tail(out))
			continue
		}
		last = nil
		break
	}
	if last != nil {
		return last
	}
	defer os.Remove(archive)
	extractDir, err := os.MkdirTemp("/opt/siroc/.cache", "ffmpeg-extract-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(extractDir) }()
	if out, err := exec.Command("tar", "-xJf", archive, "-C", extractDir, "--strip-components=1").CombinedOutput(); err != nil {
		_ = os.RemoveAll(extractDir)
		extractDir, err = os.MkdirTemp("/opt/siroc/.cache", "ffmpeg-extract-*")
		if err != nil {
			return err
		}
		if out2, err2 := exec.Command("tar", "-xJf", archive, "-C", extractDir).CombinedOutput(); err2 != nil {
			return fmt.Errorf("extract FFmpeg: %s", strings.TrimSpace(string(out)+"\n"+string(out2)))
		}
	}
	src := extractDir
	if _, err := os.Stat(filepath.Join(extractDir, "bin", "ffmpeg")); err != nil {
		if found := findFFmpegBinDir(extractDir); found != "" {
			src = found
		}
	}
	if _, err := os.Stat(filepath.Join(src, "bin", "ffmpeg")); err != nil {
		return fmt.Errorf("FFmpeg %s extract missing ffmpeg binary", version)
	}
	dest := filepath.Join(ffmpegRoot, version)
	_ = os.MkdirAll(ffmpegRoot, 0755)
	_ = os.RemoveAll(dest)
	if err := os.Rename(src, dest); err != nil {
		if err := exec.Command("cp", "-a", src, dest).Run(); err != nil {
			return err
		}
	}
	return setFFmpegCLI(version)
}

func findFFmpegBinDir(root string) string {
	var found string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if d.Name() != "ffmpeg" {
			return nil
		}
		if filepath.Base(filepath.Dir(path)) != "bin" {
			return nil
		}
		found = filepath.Dir(filepath.Dir(path))
		return io.EOF
	})
	return found
}

func ffmpegArch() string {
	out, _ := exec.Command("uname", "-m").Output()
	switch strings.TrimSpace(string(out)) {
	case "aarch64", "arm64":
		return "linuxarm64"
	default:
		return "linux64"
	}
}

func ffmpegURLs(version string) []string {
	arch := ffmpegArch()
	base := "https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/"
	return []string{
		base + "ffmpeg-n" + version + "-latest-" + arch + "-gpl-" + version + ".tar.xz",
		base + "ffmpeg-n" + version + "-latest-" + arch + "-gpl.tar.xz",
	}
}

func upgradeFFmpeg(version string) error {
	vers := []string{}
	if version != "" {
		vers = []string{version}
	} else {
		vers = ffmpegInstalledVersions()
		if len(vers) == 0 {
			vers = []string{ffmpegDefaultVersion()}
		}
	}
	for _, v := range vers {
		_ = os.RemoveAll(filepath.Join(ffmpegRoot, v))
		if err := installFFmpeg(v); err != nil {
			return err
		}
	}
	return nil
}

var ffmpegAssetRe = regexp.MustCompile(`^ffmpeg-n(\d+\.\d+)-latest-linux(?:64|arm64)-gpl-(\d+\.\d+)\.tar\.xz$`)

func fetchFFmpegVersions() []string {
	client := &http.Client{Timeout: 6 * time.Second}
	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/BtbN/FFmpeg-Builds/releases/latest", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "siroc")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var rel struct {
		Assets []struct {
			Name string `json:"name"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil
	}
	seen := map[string]struct{}{}
	var vers []string
	for _, a := range rel.Assets {
		m := ffmpegAssetRe.FindStringSubmatch(a.Name)
		if len(m) != 3 || m[1] != m[2] {
			continue
		}
		if strings.Contains(a.Name, "shared") {
			continue
		}
		if _, ok := seen[m[1]]; ok {
			continue
		}
		seen[m[1]] = struct{}{}
		vers = append(vers, m[1])
	}
	sort.Strings(vers)
	return vers
}

type ffmpegVerFile struct {
	Versions []string `json:"versions"`
}

func loadFFmpegVerCache() []string {
	b, err := os.ReadFile(ffmpegVerCache)
	if err != nil {
		return nil
	}
	var f ffmpegVerFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil
	}
	return f.Versions
}

func saveFFmpegVerCache(vers []string) {
	_ = os.MkdirAll(filepath.Dir(ffmpegVerCache), 0750)
	b, _ := json.Marshal(ffmpegVerFile{Versions: vers})
	_ = os.WriteFile(ffmpegVerCache, b, 0644)
}

func ffmpegVerCacheMtime() time.Time {
	st, err := os.Stat(ffmpegVerCache)
	if err != nil {
		return time.Time{}
	}
	return st.ModTime()
}

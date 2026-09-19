//go:build linux

package software

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const nodeRuntimeRoot = "/opt/siroc-runtimes/node"

func (m *Manager) SetCLI(name, version string) error {
	switch name {
	case "php":
		return setPHPCLI(version)
	case "python":
		return setPythonCLI(version)
	case "nodejs":
		return setNodeCLI(version)
	case "ffmpeg":
		return setFFmpegCLI(version)
	default:
		return fmt.Errorf("CLI switch is only supported for php, python, nodejs, and ffmpeg")
	}
}

func phpInstalledVersions() []string {
	var found []string
	for _, v := range []string{"8.1", "8.2", "8.3", "8.4"} {
		if _, err := os.Stat("/usr/bin/php" + v); err == nil {
			found = append(found, v)
			continue
		}
		if ok, _ := dpkgVersion("php" + v + "-cli"); ok {
			found = append(found, v)
		}
	}
	return found
}

func pythonInstalledVersions() []string {
	var found []string
	for _, v := range []string{"3.10", "3.11", "3.12", "3.13"} {
		if _, err := os.Stat("/usr/bin/python" + v); err == nil {
			found = append(found, v)
		}
	}
	return found
}

func nodeInstalledVersions() []string {
	seen := map[string]struct{}{}
	var found []string
	add := func(v string) {
		if v == "" {
			return
		}
		if _, ok := seen[v]; ok {
			return
		}
		seen[v] = struct{}{}
		found = append(found, v)
	}
	entries, _ := os.ReadDir(nodeRuntimeRoot)
	for _, e := range entries {
		if e.IsDir() {
			if _, err := os.Stat(filepath.Join(nodeRuntimeRoot, e.Name(), "bin", "node")); err == nil {
				add(e.Name())
			}
		}
	}
	if v := nodeMajorFromBin("node"); v != "" {
		add(v)
	}
	return found
}

func currentPHPCLI() string {
	out, err := exec.Command("php", "-r", "echo PHP_MAJOR_VERSION,'.',PHP_MINOR_VERSION;").CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func currentPythonCLI() string {
	for _, bin := range []string{"/usr/local/bin/python3", "/usr/local/bin/python", "python", "python3"} {
		out, err := exec.Command(bin, "-c", "import sys; print(f'{sys.version_info.major}.{sys.version_info.minor}')").CombinedOutput()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return ""
}

func currentNodeCLI() string {
	return nodeMajorFromBin("node")
}

func nodeMajorFromBin(bin string) string {
	out, err := exec.Command(bin, "-p", "process.versions.node").CombinedOutput()
	if err != nil {
		return ""
	}
	ver := strings.TrimPrefix(strings.TrimSpace(string(out)), "v")
	if i := strings.IndexByte(ver, '.'); i > 0 {
		return ver[:i]
	}
	return ver
}

func setPHPCLI(version string) error {
	switch version {
	case "8.1", "8.2", "8.3", "8.4":
	default:
		return fmt.Errorf("unsupported PHP version")
	}
	bin := "/usr/bin/php" + version
	if _, err := os.Stat(bin); err != nil {
		return fmt.Errorf("PHP %s CLI is not installed", version)
	}
	if err := alternativesSet("php", bin, nil); err != nil {
		return err
	}
	phar := "/usr/bin/phar" + version
	if _, err := os.Stat(phar); err == nil {
		_ = alternativesSet("phar", phar, nil)
	}
	return nil
}

func setPythonCLI(version string) error {
	switch version {
	case "3.10", "3.11", "3.12", "3.13":
	default:
		return fmt.Errorf("unsupported Python version")
	}
	bin := "/usr/bin/python" + version
	if _, err := os.Stat(bin); err != nil {
		return fmt.Errorf("Python %s is not installed", version)
	}
	if err := alternativesSet("python", bin, nil); err != nil {
		return err
	}
	// Keep distro /usr/bin/python3 for apt. User-facing python/python3
	// come from /usr/local/bin, which is first on a normal PATH.
	restoreDistroPython3()
	_ = os.Remove("/usr/local/bin/python")
	_ = os.Remove("/usr/local/bin/python3")
	if err := os.Symlink(bin, "/usr/local/bin/python"); err != nil {
		return err
	}
	return os.Symlink(bin, "/usr/local/bin/python3")
}

func restoreDistroPython3() {
	for _, v := range []string{"3.12", "3.11", "3.10"} {
		p := "/usr/bin/python" + v
		if _, err := os.Stat(p); err == nil {
			_ = exec.Command("update-alternatives", "--install", "/usr/bin/python3", "python3", p, "100").Run()
			_ = exec.Command("update-alternatives", "--set", "python3", p).Run()
			return
		}
	}
}

func setNodeCLI(version string) error {
	switch version {
	case "18", "20", "22":
	default:
		return fmt.Errorf("unsupported Node.js version")
	}
	adoptSystemNode()
	bin := filepath.Join(nodeRuntimeRoot, version, "bin", "node")
	npm := filepath.Join(nodeRuntimeRoot, version, "bin", "npm")
	npx := filepath.Join(nodeRuntimeRoot, version, "bin", "npx")
	if _, err := os.Stat(bin); err != nil {
		if sys, lookErr := exec.LookPath("node"); lookErr == nil && nodeMajorFromBin(sys) == version {
			bin = sys
			if p, err := exec.LookPath("npm"); err == nil {
				npm = p
			} else {
				npm = ""
			}
			if p, err := exec.LookPath("npx"); err == nil {
				npx = p
			} else {
				npx = ""
			}
		} else {
			return fmt.Errorf("Node.js %s is not installed", version)
		}
	}
	if err := alternativesSet("node", bin, nil); err != nil {
		return err
	}
	if npm != "" {
		if _, err := os.Stat(npm); err == nil {
			_ = alternativesSet("npm", npm, nil)
		}
	}
	if npx != "" {
		if _, err := os.Stat(npx); err == nil {
			_ = alternativesSet("npx", npx, nil)
		}
	}
	return nil
}

type altSlave struct {
	Name string
	Link string
	Path string
}

func altPriority(path string) int {
	base := filepath.Base(path)
	re := regexp.MustCompile(`(\d+)(?:\.(\d+))?`)
	m := re.FindStringSubmatch(base)
	if m == nil {
		// /opt/siroc-runtimes/node/22/bin/node
		re2 := regexp.MustCompile(`/node/(\d+)/`)
		if m2 := re2.FindStringSubmatch(path); m2 != nil {
			n, _ := strconv.Atoi(m2[1])
			return n * 10
		}
		return 1
	}
	maj, _ := strconv.Atoi(m[1])
	min := 0
	if m[2] != "" {
		min, _ = strconv.Atoi(m[2])
	}
	return maj*10 + min
}

func adoptSystemNode() {
	st, err := os.Lstat("/usr/bin/node")
	if err != nil || st.Mode()&os.ModeSymlink != 0 {
		return
	}
	major := nodeMajorFromBin("/usr/bin/node")
	if major == "" {
		_ = os.Rename("/usr/bin/node", "/usr/bin/node.cp-distro")
		displaceRegularFile("/usr/bin/npm")
		displaceRegularFile("/usr/bin/npx")
		return
	}
	destBin := filepath.Join(nodeRuntimeRoot, major, "bin")
	if _, err := os.Stat(filepath.Join(destBin, "node")); err != nil {
		_ = os.MkdirAll(destBin, 0755)
		_ = os.Rename("/usr/bin/node", filepath.Join(destBin, "node"))
		for _, tool := range []string{"npm", "npx", "corepack"} {
			src := "/usr/bin/" + tool
			if st, err := os.Lstat(src); err == nil && st.Mode()&os.ModeSymlink == 0 {
				_ = os.Rename(src, filepath.Join(destBin, tool))
			}
		}
		return
	}
	displaceRegularFile("/usr/bin/node")
	displaceRegularFile("/usr/bin/npm")
	displaceRegularFile("/usr/bin/npx")
}

func displaceRegularFile(path string) {
	st, err := os.Lstat(path)
	if err != nil || st.Mode()&os.ModeSymlink != 0 {
		return
	}
	_ = os.Rename(path, path+".cp-distro")
}

func alternativesSet(name, path string, slaves []altSlave) error {
	link := "/usr/bin/" + name
	displaceRegularFile(link)
	args := []string{"--install", link, name, path, strconv.Itoa(altPriority(path))}
	for _, s := range slaves {
		if _, err := os.Stat(s.Path); err != nil {
			continue
		}
		displaceRegularFile(s.Link)
		args = append(args, "--slave", s.Link, s.Name, s.Path)
	}
	if out, err := exec.Command("update-alternatives", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("update-alternatives install %s: %s", name, strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("update-alternatives", "--set", name, path).CombinedOutput(); err != nil {
		return fmt.Errorf("update-alternatives set %s: %s", name, strings.TrimSpace(string(out)))
	}
	return nil
}

func installNode(version string) error {
	switch version {
	case "", "22":
		version = "22"
	case "18", "20":
	default:
		return fmt.Errorf("unsupported Node.js version")
	}
	adoptSystemNode()
	dest := filepath.Join(nodeRuntimeRoot, version)
	if _, err := os.Stat(filepath.Join(dest, "bin", "node")); err == nil {
		RepairNodeRuntimes()
		return setNodeCLI(version)
	}
	url, err := latestNodeTarball(version)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "node-*.tar.xz")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := downloadFile(url, tmp); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	if err := os.MkdirAll(nodeRuntimeRoot, 0755); err != nil {
		return err
	}
	extractDir, err := os.MkdirTemp("", "node-extract-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(extractDir)
	if out, err := exec.Command("tar", "-xJf", tmp.Name(), "-C", extractDir, "--strip-components=1").CombinedOutput(); err != nil {
		return fmt.Errorf("extract node: %s: %w", strings.TrimSpace(string(out)), err)
	}
	_ = os.RemoveAll(dest)
	if err := os.Rename(extractDir, dest); err != nil {
		if err := exec.Command("cp", "-a", extractDir, dest).Run(); err != nil {
			return err
		}
	}
	chmodWorldRX(dest)
	RepairNodeRuntimes()
	return setNodeCLI(version)
}

func RepairNodeRuntimes() {
	_ = os.MkdirAll(nodeRuntimeRoot, 0755)
	_ = os.Chmod(nodeRuntimeRoot, 0755)
	entries, err := os.ReadDir(nodeRuntimeRoot)
	if err != nil {
		return
	}
	var npmSrc string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(nodeRuntimeRoot, e.Name())
		chmodWorldRX(dir)
		if fileExists(filepath.Join(dir, "bin", "npm")) {
			npmSrc = dir
		}
	}
	if npmSrc == "" {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dest := filepath.Join(nodeRuntimeRoot, e.Name())
		if !fileExists(filepath.Join(dest, "bin", "node")) || fileExists(filepath.Join(dest, "bin", "npm")) {
			continue
		}
		_ = attachNodeNPM(dest, npmSrc)
	}
}

func attachNodeNPM(dest, src string) error {
	srcMod := filepath.Join(src, "lib", "node_modules", "npm")
	destMod := filepath.Join(dest, "lib", "node_modules", "npm")
	if !fileExists(srcMod) {
		return fmt.Errorf("npm not found in %s", src)
	}
	if !fileExists(destMod) {
		if err := os.MkdirAll(filepath.Dir(destMod), 0755); err != nil {
			return err
		}
		if err := exec.Command("cp", "-a", srcMod, destMod).Run(); err != nil {
			return fmt.Errorf("copy npm: %w", err)
		}
	}
	links := map[string]string{
		"npm": "../lib/node_modules/npm/bin/npm-cli.js",
		"npx": "../lib/node_modules/npm/bin/npx-cli.js",
	}
	for name, rel := range links {
		link := filepath.Join(dest, "bin", name)
		if fileExists(link) {
			continue
		}
		if _, err := os.Stat(filepath.Join(dest, "bin", rel)); err != nil {
			continue
		}
		_ = os.Symlink(rel, link)
	}
	chmodWorldRX(dest)
	return nil
}

func chmodWorldRX(root string) {
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		mode := info.Mode()
		if d.IsDir() || mode&0111 != 0 {
			_ = os.Chmod(path, (mode&0777)|0755)
		} else {
			_ = os.Chmod(path, (mode&0777)|0644)
		}
		return nil
	})
}

func latestNodeTarball(major string) (string, error) {
	base := fmt.Sprintf("https://nodejs.org/dist/latest-v%s.x/", major)
	client := &http.Client{Timeout: 45 * time.Second}
	res, err := client.Get(base + "SHASUMS256.txt")
	if err != nil {
		return "", fmt.Errorf("lookup Node.js %s: %w", major, err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return "", fmt.Errorf("lookup Node.js %s: HTTP %d", major, res.StatusCode)
	}
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`node-v[0-9.]+-linux-x64\.tar\.xz`)
	name := re.FindString(string(b))
	if name == "" {
		return "", fmt.Errorf("could not find linux-x64 tarball for Node.js %s", major)
	}
	return base + name, nil
}

func downloadFile(url string, dest *os.File) error {
	client := &http.Client{Timeout: 4 * time.Minute}
	res, err := client.Get(url)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return fmt.Errorf("download %s: HTTP %d", url, res.StatusCode)
	}
	_, err = io.Copy(dest, res.Body)
	return err
}

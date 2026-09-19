//go:build linux

package software

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/validate"
)

const (
	cliMarkerBegin = "# BEGIN SIROC-CLI"
	cliMarkerEnd   = "# END SIROC-CLI"
	cliMarkerBeginOld = "# BEGIN CP-CLI"
	cliMarkerEndOld   = "# END CP-CLI"
	profileSnippet = `# BEGIN SIROC-CLI
if [ -f "$HOME/.siroc/cli.env" ]; then
  . "$HOME/.siroc/cli.env"
fi
# END SIROC-CLI
`
)

func (m *Manager) ApplyUserCLI(req rpc.UserCLIReq) error {
	if err := validate.LinuxUser(req.Username); err != nil {
		return err
	}
	u, err := user.Lookup(req.Username)
	if err != nil {
		return fmt.Errorf("unknown linux user")
	}
	home, err := filepath.Abs(u.HomeDir)
	if err != nil {
		return err
	}
	bins := map[string]string{}
	if req.PHP != "" {
		p, err := resolveCLIBin("php", req.PHP)
		if err != nil {
			return err
		}
		bins["php"] = p
	}
	if req.Python != "" {
		p, err := resolveCLIBin("python", req.Python)
		if err != nil {
			return err
		}
		bins["python"] = p
		bins["python3"] = p
	}
	if req.Node != "" {
		p, err := resolveCLIBin("nodejs", req.Node)
		if err != nil {
			return err
		}
		bins["node"] = p
		if npm := nodeToolBeside(p, "npm"); npm != "" {
			bins["npm"] = npm
		}
		if npx := nodeToolBeside(p, "npx"); npx != "" {
			bins["npx"] = npx
		}
	}

	localBin := filepath.Join(home, ".local", "bin")
	sirocDir := filepath.Join(home, ".siroc")
	if err := os.MkdirAll(localBin, 0750); err != nil {
		return err
	}
	if err := os.MkdirAll(sirocDir, 0750); err != nil {
		return err
	}
	for _, name := range []string{"php", "python", "python3", "node", "npm", "npx"} {
		_ = os.Remove(filepath.Join(localBin, name))
	}
	for name, target := range bins {
		if err := writeWrapper(filepath.Join(localBin, name), target); err != nil {
			return err
		}
	}
	env := "export PATH=\"$HOME/.local/bin:$PATH\"\n"
	if err := os.WriteFile(filepath.Join(sirocDir, "cli.env"), []byte(env), 0640); err != nil {
		return err
	}
	for _, rc := range []string{".bashrc", ".profile", ".bash_profile"} {
		if err := ensureSourceSnippet(filepath.Join(home, rc)); err != nil {
			return err
		}
	}
	_ = os.Remove("/etc/profile.d/cp-user-cli.sh")
	if err := os.WriteFile("/etc/profile.d/siroc-user-cli.sh", []byte(profileSnippet+"\n"), 0644); err != nil {
		return err
	}
	_ = exec.Command("chown", "-R", req.Username+":"+req.Username, filepath.Join(home, ".local")).Run()
	_ = exec.Command("chown", "-R", req.Username+":"+req.Username, sirocDir).Run()
	for _, rc := range []string{".bashrc", ".profile", ".bash_profile"} {
		p := filepath.Join(home, rc)
		if _, err := os.Stat(p); err == nil {
			_ = exec.Command("chown", req.Username+":"+req.Username, p).Run()
		}
	}
	return nil
}

func nodeToolBeside(nodeBin, name string) string {
	p := filepath.Join(filepath.Dir(nodeBin), name)
	if fileExists(p) {
		return p
	}
	entries, err := os.ReadDir(nodeRuntimeRoot)
	if err != nil {
		return ""
	}
	for i := len(entries) - 1; i >= 0; i-- {
		cand := filepath.Join(nodeRuntimeRoot, entries[i].Name(), "bin", name)
		if fileExists(cand) {
			return cand
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return ""
}

func resolveCLIBin(name, version string) (string, error) {
	switch name {
	case "php":
		switch version {
		case "8.1", "8.2", "8.3", "8.4":
		default:
			return "", fmt.Errorf("unsupported PHP version")
		}
		bin := "/usr/bin/php" + version
		if !fileExists(bin) {
			return "", fmt.Errorf("PHP %s CLI is not installed", version)
		}
		return bin, nil
	case "python":
		switch version {
		case "3.10", "3.11", "3.12", "3.13":
		default:
			return "", fmt.Errorf("unsupported Python version")
		}
		bin := "/usr/bin/python" + version
		if !fileExists(bin) {
			return "", fmt.Errorf("Python %s is not installed", version)
		}
		return bin, nil
	case "nodejs":
		switch version {
		case "18", "20", "22":
		default:
			return "", fmt.Errorf("unsupported Node.js version")
		}
		bin := filepath.Join(nodeRuntimeRoot, version, "bin", "node")
		if fileExists(bin) {
			return bin, nil
		}
		if sys, err := exec.LookPath("node"); err == nil && nodeMajorFromBin(sys) == version {
			return sys, nil
		}
		return "", fmt.Errorf("Node.js %s is not installed", version)
	default:
		return "", fmt.Errorf("unknown runtime")
	}
}

func writeWrapper(path, target string) error {
	body := "#!/bin/sh\nexec " + target + " \"$@\"\n"
	if err := os.WriteFile(path, []byte(body), 0750); err != nil {
		return err
	}
	return nil
}

func ensureSourceSnippet(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return os.WriteFile(path, []byte(profileSnippet+"\n"), 0640)
		}
		return err
	}
	s := string(b)
	for _, pair := range [][2]string{{cliMarkerBegin, cliMarkerEnd}, {cliMarkerBeginOld, cliMarkerEndOld}} {
		if strings.Contains(s, pair[0]) {
			start := strings.Index(s, pair[0])
			end := strings.Index(s, pair[1])
			if start >= 0 && end > start {
				s = s[:start] + profileSnippet + s[end+len(pair[1]):]
				return os.WriteFile(path, []byte(s), 0640)
			}
		}
	}
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return os.WriteFile(path, []byte(s+profileSnippet+"\n"), 0640)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

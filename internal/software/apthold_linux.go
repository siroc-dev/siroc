//go:build linux

package software

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// EnsureAptHolds marks packages the panel installed so apt upgrade leaves them.
// The first run records software that is already installed. Later runs only
// refresh that list.
func EnsureAptHolds() (int, error) {
	if _, err := exec.LookPath("apt-mark"); err != nil {
		return 0, nil
	}
	installed := listInstalledDebs()
	if _, err := os.Stat(AptManagedPath); os.IsNotExist(err) {
		if err := writeManaged(seedManagedPackages(installed)); err != nil {
			return 0, err
		}
	}
	cur := readManaged()
	next := expandHold(cur, installed)
	if err := writeManaged(next); err != nil {
		return 0, err
	}
	if err := aptMark("hold", next); err != nil {
		return len(next), err
	}
	return len(next), nil
}

// ManagedAptCount is how many packages apt upgrade will skip.
func ManagedAptCount() int {
	return len(readManaged())
}

func seedManagedPackages(installed []string) []string {
	var seeds []string
	for _, s := range catalog() {
		if s.Name == "python" || s.Name == "openssh" {
			continue
		}
		ok, _ := s.Installed()
		if !ok {
			continue
		}
		for _, name := range aptSeedNames(s.Name) {
			if ShouldSeed(name) {
				seeds = append(seeds, name)
			}
		}
	}
	return expandHold(seeds, installed)
}

func aptSeedNames(name string) []string {
	switch name {
	case "nginx":
		return []string{"nginx"}
	case "apache":
		return []string{"apache2"}
	case "php":
		var out []string
		for _, v := range phpInstalledVersions() {
			out = append(out, "php"+v+"-fpm")
		}
		return out
	case "mysql":
		return []string{"mysql-community-server", "mysql-server"}
	case "mariadb":
		return []string{"mariadb-server"}
	case "redis":
		return []string{"redis-server"}
	case "python":
		var out []string
		for _, v := range pythonInstalledVersions() {
			out = append(out, "python"+v)
		}
		return out
	case "nodejs", "ffmpeg", "composer", "wp-cli", "phpmyadmin", "zap", "openvas":
		return nil
	default:
		for _, s := range catalog() {
			if s.Name == name && s.AptPkg != "" {
				return strings.Fields(s.AptPkg)
			}
		}
	}
	return nil
}

func protectApt(pkgs ...string) {
	var seeds []string
	for _, p := range pkgs {
		if panelManagedPkg(p) {
			seeds = append(seeds, p)
		}
	}
	if len(seeds) == 0 {
		return
	}
	installed := listInstalledDebs()
	add := expandHold(seeds, installed)
	if len(add) == 0 {
		return
	}
	next := normalizePkgs(append(readManaged(), add...))
	if err := writeManaged(next); err != nil {
		noteInstall("apt hold failed: " + err.Error())
		return
	}
	if err := aptMark("hold", add); err != nil {
		noteInstall("apt hold failed: " + err.Error())
		return
	}
	noteInstall("apt hold: " + strings.Join(add, ", "))
}

func releasePanel(pkgs []string) {
	if len(pkgs) == 0 {
		return
	}
	names := releaseNames(pkgs, readManaged(), listInstalledDebs())
	_ = aptMark("unhold", names)
}

func holdRecorded() {
	installed := map[string]struct{}{}
	for _, name := range listInstalledDebs() {
		installed[name] = struct{}{}
	}
	var pkgs []string
	for _, name := range readManaged() {
		if _, ok := installed[name]; ok {
			pkgs = append(pkgs, name)
		}
	}
	if err := aptMark("hold", pkgs); err != nil {
		noteInstall("apt hold failed: " + err.Error())
	}
}

func readManaged() []string {
	b, err := os.ReadFile(AptManagedPath)
	if err != nil {
		return nil
	}
	return parseManaged(string(b))
}

func writeManaged(pkgs []string) error {
	if err := os.MkdirAll("/var/lib/siroc", 0755); err != nil {
		return err
	}
	return os.WriteFile(AptManagedPath, []byte(formatManaged(pkgs)), 0644)
}

func listInstalledDebs() []string {
	out, err := exec.Command("dpkg-query", "-W", "-f", "${Package}\t${Status}\n").Output()
	if err != nil {
		return nil
	}
	var pkgs []string
	for _, line := range strings.Split(string(out), "\n") {
		name, status, ok := strings.Cut(line, "\t")
		if !ok || !strings.Contains(status, "install ok installed") {
			continue
		}
		name = strings.TrimSpace(name)
		if name != "" {
			pkgs = append(pkgs, name)
		}
	}
	return pkgs
}

func aptMark(action string, pkgs []string) error {
	pkgs = normalizePkgs(pkgs)
	if len(pkgs) == 0 {
		return nil
	}
	if _, err := exec.LookPath("apt-mark"); err != nil {
		return nil
	}
	var last error
	for i := 0; i < len(pkgs); i += 80 {
		end := i + 80
		if end > len(pkgs) {
			end = len(pkgs)
		}
		cmd := exec.Command("apt-mark", append([]string{action}, pkgs[i:end]...)...)
		cmd.Env = aptEnv()
		out, err := cmd.CombinedOutput()
		if err != nil && action == "hold" {
			last = fmt.Errorf("apt-mark %s: %s", action, strings.TrimSpace(string(out)))
		}
	}
	return last
}

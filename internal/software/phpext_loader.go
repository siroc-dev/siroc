package software

import (
	"fmt"
	"strings"
)

func parseExtensionDir(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		dir := strings.TrimSpace(lines[i])
		if strings.HasPrefix(dir, "/") && !strings.ContainsAny(dir, " \t") {
			return dir
		}
	}
	return ""
}

func ioncubeLoaderSO(version string) string {
	return "ioncube_loader_lin_" + version + ".so"
}

func ioncubeArchiveURL(arch string) (string, error) {
	switch arch {
	case "x86_64", "amd64":
		return "https://downloads.ioncube.com/loader_downloads/ioncube_loaders_lin_x86-64.tar.gz", nil
	case "aarch64", "arm64":
		return "https://downloads.ioncube.com/loader_downloads/ioncube_loaders_lin_aarch64.tar.gz", nil
	default:
		return "", fmt.Errorf("ionCube has no loader for %s", arch)
	}
}

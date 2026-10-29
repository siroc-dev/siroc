package hosting

import "strings"

func useCustomLaravelRoot(action, focus string) bool {
	if strings.TrimSpace(focus) == "wordpress" {
		return false
	}
	switch strings.TrimSpace(action) {
	case "queue", "scheduler", "composer", "npm", "artisan", "env-get", "env-set", "packages", "pkg-search":
		return true
	case "", "status":
		return strings.TrimSpace(focus) == "laravel"
	default:
		return false
	}
}

func pickLaravelRoot(detectedKind, detectedRoot, customAbs string, hasArtisan bool) (string, string, bool) {
	if hasArtisan {
		return "laravel", customAbs, false
	}
	if detectedKind == "wordpress" {
		return detectedKind, detectedRoot, false
	}
	return "php", customAbs, true
}

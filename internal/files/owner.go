package files

import (
	"path/filepath"
	"strings"
)

func accountFromHomePath(homeRoot, abs string) (name string, ok bool) {
	if homeRoot == "" {
		homeRoot = "/home"
	}
	homeRoot = filepath.Clean(homeRoot)
	abs = filepath.Clean(abs)
	rel, err := filepath.Rel(homeRoot, abs)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, "..") {
		return "", false
	}
	name = strings.Split(filepath.ToSlash(rel), "/")[0]
	if name == "" || name == "." {
		return "", false
	}
	return name, true
}

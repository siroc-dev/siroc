package files

import (
	"path/filepath"
	"testing"
)

func TestAccountFromHomePath(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "home")
	inside := filepath.Join(root, "jaijai", "domains", "siamdriedberry.shop", "public_html", "vendor")
	name, ok := accountFromHomePath(root, inside)
	if !ok || name != "jaijai" {
		t.Fatalf("got %q %v", name, ok)
	}
	if name, ok := accountFromHomePath(root, root); ok || name != "" {
		t.Fatalf("home root itself should not map to a user: %q %v", name, ok)
	}
	if name, ok := accountFromHomePath(root, filepath.Join(string(filepath.Separator), "etc", "nginx")); ok {
		t.Fatalf("system path mapped to %q", name)
	}
}

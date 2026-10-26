package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirBytesSumsFilesInTheFolder(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.txt"), []byte("world!"), 0644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(sub, "nested")
	if err := os.Mkdir(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "c.txt"), []byte("ab"), 0644); err != nil {
		t.Fatal(err)
	}
	n, err := dirBytes(sub)
	if err != nil || n != 8 {
		t.Fatalf("size %d %v", n, err)
	}
	if !skipDirSize("/proc") || !skipDirSize("/proc/1") || skipDirSize(sub) {
		t.Fatal("skip")
	}
}

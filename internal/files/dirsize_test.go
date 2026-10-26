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

func TestFollowsDirTreatsSymlinkFolderAsDir(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.Mkdir(real, 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skip(err)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if !followsDir(link, info) {
		t.Fatal("symlink folder")
	}
	file := filepath.Join(root, "a.txt")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(file)
	if err != nil || followsDir(file, fi) {
		t.Fatalf("file %v", err)
	}
	if err := os.WriteFile(filepath.Join(real, "a.txt"), []byte("hello!!!"), 0644); err != nil {
		t.Fatal(err)
	}
	n, err := dirBytes(link)
	if err != nil || n != 8 {
		t.Fatalf("symlink folder size %d %v", n, err)
	}
}

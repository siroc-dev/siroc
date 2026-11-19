package api

import (
	"testing"

	"github.com/siroc-dev/siroc/internal/store"
)

func TestPickLocalDest(t *testing.T) {
	home := store.BackupDest{Kind: "local", Path: "/home/alice/backups", Name: "Home"}
	server := store.BackupDest{Kind: "local", Path: "/var/backups/siroc/", Name: "Server archives"}
	ftp := store.BackupDest{Kind: "ftp", Path: "/backups", Name: "FTP"}
	got := pickLocalDest([]store.BackupDest{home, ftp, server})
	if got == nil || got.Name != "Server archives" {
		t.Fatalf("prefer server archives, got %#v", got)
	}
	got = pickLocalDest([]store.BackupDest{ftp, home})
	if got == nil || got.Path != "/home/alice/backups" {
		t.Fatalf("fallback local, got %#v", got)
	}
	if pickLocalDest([]store.BackupDest{ftp}) != nil {
		t.Fatal("ftp is not a local destination")
	}
}

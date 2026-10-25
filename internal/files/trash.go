package files

import (
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/layout"
)

const (
	TrashDirName  = ".siroc-trash"
	TrashKeepDays = 15
	TrashKeep     = TrashKeepDays * 24 * time.Hour
)

// RootTrashDir holds files deleted from the root file manager.
const RootTrashDir = layout.Data + "/file-trash"

var trashIDPattern = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}\.[0-9]{9}(?:-[0-9]+)?$`)

type trashMeta struct {
	Name    string    `json:"name"`
	From    string    `json:"from"`
	Deleted time.Time `json:"deleted"`
	IsDir   bool      `json:"isDir"`
}

func newTrashID(now time.Time) string {
	return now.UTC().Format("20060102T150405.000000000")
}

func validTrashID(id string) bool {
	return trashIDPattern.MatchString(id)
}

func trashExpired(deleted, now time.Time) bool {
	if deleted.IsZero() {
		return false
	}
	return !now.Before(deleted.UTC().Add(TrashKeep))
}

func trashExpires(deleted time.Time) time.Time {
	return deleted.UTC().Add(TrashKeep)
}

// trashDestOK reports whether dest is one item directory inside a trash root.
func trashDestOK(home, dest string) bool {
	dest = slashClean(dest)
	if !validTrashID(path.Base(dest)) {
		return false
	}
	return path.Dir(dest) == trashRootFor(home)
}

func trashRootFor(home string) string {
	home = slashClean(home)
	if home == "" || home == "/" || home == "." {
		return slashClean(RootTrashDir)
	}
	return strings.TrimRight(home, "/") + "/" + TrashDirName
}

func trashHolds(home, abs string) bool {
	abs = slashClean(abs)
	root := trashRootFor(home)
	return abs == root || within(root, abs)
}

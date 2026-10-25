package files

import (
	"testing"
	"time"
)

func TestTrashKeepIs15Days(t *testing.T) {
	deleted := time.Date(2026, 10, 25, 2, 0, 0, 0, time.UTC)
	if trashExpired(deleted, deleted.Add(15*24*time.Hour-time.Second)) {
		t.Fatal("kept until the 15th day")
	}
	if !trashExpired(deleted, deleted.Add(15*24*time.Hour)) {
		t.Fatal("expires after 15 days")
	}
	if !trashExpires(deleted).Equal(deleted.Add(TrashKeep)) {
		t.Fatal(trashExpires(deleted))
	}
}

func TestValidTrashID(t *testing.T) {
	id := newTrashID(time.Date(2026, 10, 25, 2, 30, 4, 123456789, time.UTC))
	if id != "20261025T023004.123456789" || !validTrashID(id) {
		t.Fatal(id)
	}
	if !validTrashID(id + "-2") {
		t.Fatal("suffix")
	}
	for _, bad := range []string{"", "..", "../etc", ".siroc-trash", "20261025", id + "/payload"} {
		if validTrashID(bad) {
			t.Fatal(bad)
		}
	}
}

func TestTrashDestOK(t *testing.T) {
	id := "20261025T023004.123456789"
	home := "/home/alice"
	if !trashDestOK(home, home+"/.siroc-trash/"+id) {
		t.Fatal("account item")
	}
	if trashDestOK(home, home+"/.siroc-trash") || trashDestOK(home, home+"/public_html/"+id) {
		t.Fatal("not an item directory")
	}
	if !trashDestOK("/", RootTrashDir+"/"+id) {
		t.Fatal("root item")
	}
	if !trashHolds(home, home+"/.siroc-trash/"+id+"/payload/a.php") {
		t.Fatal("inside trash")
	}
	if trashHolds(home, home+"/public_html/a.php") {
		t.Fatal("live file")
	}
}

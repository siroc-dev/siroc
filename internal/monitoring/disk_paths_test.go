package monitoring

import (
	"reflect"
	"testing"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func TestSiteDiskPaths(t *testing.T) {
	items := []rpc.SiteDiskItem{
		{User: "alice", Path: "domains/a.test/public_html"},
		{User: "alice", Path: "/home/alice/domains/a.test/public_html"},
		{User: "alice", Path: "/mnt/data/alice"},
		{User: "alice", Path: "/mnt/data/alice/public"},
		{User: "alice", Path: "/mnt/data/alice"},
		{User: "bob", Path: "/mnt/other/bob"},
		{User: "bob", Path: "/mnt/database"},
		{User: "carol", Path: "/home/carol"},
	}
	got := SiteDiskPaths("/home", items)
	want := map[string][]string{
		"alice": {"/mnt/data/alice"},
		"bob":   {"/mnt/database", "/mnt/other/bob"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestParseDu(t *testing.T) {
	got := parseDu("120\t/mnt/data/alice\n40 /mnt/other/bob\n\nbad\n")
	want := map[string]uint64{
		"/mnt/data/alice": 120,
		"/mnt/other/bob":  40,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
}

func TestDiskCacheFresh(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	if diskCacheFresh(time.Time{}, now) {
		t.Fatal("zero time is not a finished scan")
	}
	if !diskCacheFresh(now.Add(-30*time.Minute), now) {
		t.Fatal("a scan from this hour should be reused")
	}
	if diskCacheFresh(now.Add(-diskInterval), now) {
		t.Fatal("a scan an hour old should refresh")
	}
}

func TestSiteDiskRequestKey(t *testing.T) {
	a := siteDiskRequestKey("/home", []rpc.SiteDiskItem{
		{User: "bob", Path: "/mnt/b"},
		{User: "alice", Path: "/mnt/a"},
	})
	b := siteDiskRequestKey("/home/", []rpc.SiteDiskItem{
		{User: "alice", Path: "/mnt/a"},
		{User: "bob", Path: "/mnt/b"},
	})
	if a != b {
		t.Fatalf("key not stable: %q vs %q", a, b)
	}
	if a == siteDiskRequestKey("/home", []rpc.SiteDiskItem{{User: "alice", Path: "/mnt/other"}}) {
		t.Fatal("different paths should not share a key")
	}
}

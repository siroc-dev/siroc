package monitoring

import (
	"reflect"
	"testing"

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

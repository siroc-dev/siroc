package software

import (
	"testing"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func TestMysqlOn(t *testing.T) {
	if !mysqlOn("ON") || !mysqlOn("1") || mysqlOn("OFF") {
		t.Fatal("mysqlOn")
	}
}

func TestParseSlowDigest(t *testing.T) {
	raw := "shop\t42\t12.5\t80\t900\t2026-10-02 01:00:00\tSELECT * FROM orders WHERE id = ?\nNULL\t3\t2000\t4000\t10\t2026-10-01 00:00:00\tSELECT sleep(?)"
	got := parseSlowDigest(raw)
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].Schema != "shop" || got[0].Count != 42 || got[0].AvgMs != 12.5 || got[0].Query != "SELECT * FROM orders WHERE id = ?" {
		t.Fatalf("%#v", got[0])
	}
	if got[1].Schema != "" || got[1].MaxMs != 4000 {
		t.Fatalf("%#v", got[1])
	}
}

func TestSlowRunning(t *testing.T) {
	list := []rpc.DBProcess{
		{ID: 1, Command: "Sleep", Time: 40, Info: ""},
		{ID: 2, Command: "Query", Time: 8, Info: "SELECT * FROM big"},
		{ID: 3, Command: "Query", Time: 1, Info: "SELECT 1"},
	}
	got := slowRunning(list, 2)
	if len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("%#v", got)
	}
}

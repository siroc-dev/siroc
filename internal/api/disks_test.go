package api

import (
	"testing"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func TestDashboardDisksSplitsMounts(t *testing.T) {
	got := dashboardDisks([]rpc.SystemDisk{
		{Mount: "/mnt/data", Device: "/dev/sdb1", Total: 100, Used: 40},
		{Mount: "/home", Device: "/dev/sda1", Total: 900, Used: 30},
		{Mount: "/", Device: "/dev/sda1", Total: 900, Used: 30},
		{Mount: "/boot", Device: "/dev/sda2", Total: 2, Used: 1},
	})
	if len(got) != 3 {
		t.Fatalf("len %d", len(got))
	}
	if got[0].Mount != "/" || got[1].Mount != "/boot" || got[2].Mount != "/mnt/data" {
		t.Fatalf("order %s %s %s", got[0].Mount, got[1].Mount, got[2].Mount)
	}
	if got[0].Total != 900 || got[2].Total != 100 {
		t.Fatalf("sizes %d %d", got[0].Total, got[2].Total)
	}
}

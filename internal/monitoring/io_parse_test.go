package monitoring

import "testing"

func TestParseDiskstats(t *testing.T) {
	raw := `   8       0 sda 100 0 200 0 10 0 40 0 0 50 0
   8       1 sda1 1 0 2 0 1 0 2 0 0 1 0
 259       0 nvme0n1 50 0 80 0 20 0 30 0 0 12 0
 259       1 nvme0n1p1 4 0 8 0 2 0 4 0 0 1 0
   7       0 loop0 0 0 0 0 0 0 0 0 0 0 0
`
	got := parseDiskstats(raw)
	if len(got) != 2 {
		t.Fatalf("len=%d %#v", len(got), got)
	}
	if got[0].name != "sda" || got[0].reads != 100 || got[0].rsect != 200 || got[0].writes != 10 || got[0].wsect != 40 || got[0].ioms != 50 {
		t.Fatalf("%#v", got[0])
	}
	if got[1].name != "nvme0n1" {
		t.Fatalf("%#v", got[1])
	}
}

func TestIsWholeDisk(t *testing.T) {
	if !isWholeDisk("vda") || !isWholeDisk("nvme0n1") || isWholeDisk("sda1") || isWholeDisk("nvme0n1p2") || isWholeDisk("loop0") {
		t.Fatal("isWholeDisk")
	}
}

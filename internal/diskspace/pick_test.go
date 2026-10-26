package diskspace

import "testing"

func TestPickSkipsMemoryAndUsesLargestDisk(t *testing.T) {
	got := Pick([]Mount{
		{Avail: 9 << 30, Type: "tmpfs", Path: "/tmp"},
		{Avail: 20 << 30, Type: "ext4", Path: "/"},
		{Avail: 80 << 30, Type: "xfs", Path: "/mnt/data"},
		{Avail: 100 << 30, Type: "overlay", Path: "/var/lib/docker"},
	})
	if got != "/mnt/data" {
		t.Fatal(got)
	}
	if tmpOn("/") != "/siroc-tmp" || tmpOn("/mnt/data") != "/mnt/data/siroc-tmp" {
		t.Fatal(tmpOn("/"), tmpOn("/mnt/data"))
	}
}

func TestParseDF(t *testing.T) {
	text := "Avail Type Target\n100 ext4 /\n50 tmpfs /tmp\n"
	got := ParseDF(text)
	if len(got) != 2 || got[0].Path != "/" || got[0].Avail != 100 || got[1].Type != "tmpfs" {
		t.Fatalf("%#v", got)
	}
}

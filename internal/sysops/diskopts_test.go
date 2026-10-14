package sysops

import "testing"

func TestCleanDiskOptions(t *testing.T) {
	fs, opt, err := CleanDiskOptions("ext4", "noatime,nosuid,rw")
	if err != nil {
		t.Fatal(err)
	}
	if fs != "ext4" || opt != "rw,noatime,nosuid" {
		t.Fatalf("got %s %s", fs, opt)
	}
	if _, _, err := CleanDiskOptions("ext4", "rw,ro"); err == nil {
		t.Fatal("both access modes accepted")
	}
	if _, _, err := CleanDiskOptions("ext4", "exec"); err == nil {
		t.Fatal("unknown option accepted")
	}
	fs, opt, err = CleanDiskOptions("", "")
	if err != nil || fs != "" || opt != "defaults" {
		t.Fatalf("detect: %s %s %v", fs, opt, err)
	}
	fs, opt, err = CleanDiskOptions("ntfs", "discard")
	if err != nil || fs != "ntfs3" || opt != "discard" {
		t.Fatalf("ntfs: %s %s %v", fs, opt, err)
	}
	fs, opt, err = CleanDiskOptions("swap", "rw")
	if err != nil || fs != "swap" || opt != "sw" {
		t.Fatalf("swap: %s %s %v", fs, opt, err)
	}
}

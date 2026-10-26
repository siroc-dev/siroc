package sysops

import "testing"

const lsblkSample = `{
  "blockdevices": [
    {
      "name": "sda",
      "path": "/dev/sda",
      "type": "disk",
      "size": 50000000000,
      "fstype": null,
      "mountpoint": null,
      "children": [
        {
          "name": "sda1",
          "path": "/dev/sda1",
          "type": "part",
          "size": 49900000000,
          "fstype": "ext4",
          "mountpoint": "/"
        }
      ]
    },
    {
      "name": "sdb",
      "path": "/dev/sdb",
      "type": "disk",
      "size": 1000000000000,
      "fstype": null,
      "mountpoint": null,
      "model": "SAMSUNG",
      "tran": "sata"
    },
    {
      "name": "sdc",
      "path": "/dev/sdc",
      "type": "disk",
      "size": 1000000000000,
      "fstype": null,
      "mountpoint": null
    },
    {
      "name": "md0",
      "path": "/dev/md0",
      "type": "raid1",
      "size": 500000000000,
      "fstype": "ext4",
      "mountpoint": "/mnt/data"
    }
  ]
}`

func TestClassifyBlockDevicesSkipsTheSystemDisk(t *testing.T) {
	md := "md0 : active raid1 sdd[0] sde[1]\n"
	st := ClassifyBlockDevices([]byte(lsblkSample), md)
	if len(st.Disks) != 2 {
		t.Fatalf("unused %#v", st.Disks)
	}
	if st.Disks[0].Path != "/dev/sdb" || st.Disks[1].Path != "/dev/sdc" {
		t.Fatalf("paths %#v", st.Disks)
	}
	if len(st.Arrays) != 1 || st.Arrays[0].Device != "/dev/md0" || st.Arrays[0].State != "active" {
		t.Fatalf("arrays %#v", st.Arrays)
	}
	if _, _, _, err := CheckRAIDCreate("1", []string{"/dev/sda1"}, st.Disks, st.Arrays); err == nil {
		t.Fatal("system partition")
	}
	level, name, devices, err := CheckRAIDCreate("1", []string{"/dev/sdb", "/dev/sdc"}, st.Disks, st.Arrays)
	if err != nil || level != "1" || name != "md1" || len(devices) != 2 {
		t.Fatalf("%s %s %v %v", level, name, devices, err)
	}
}

func TestRAIDRules(t *testing.T) {
	if ValidRAIDDevice("/dev/sda") && ValidRAIDDevice("/dev/nvme0n1p2") && !ValidRAIDDevice("/dev/sda/../sdb") {
	} else {
		t.Fatal("device")
	}
	if RAIDMinDisks("5") != 3 || RAIDMinDisks("9") != 0 {
		t.Fatal("min")
	}
	if _, err := NormalizeRAIDLevel("raid10"); err != nil {
		t.Fatal(err)
	}
}

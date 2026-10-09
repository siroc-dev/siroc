package sysops

import "testing"

func TestVLANIfaceName(t *testing.T) {
	name, err := vlanIfaceName("eth0", 100)
	if err != nil || name != "eth0.100" {
		t.Fatalf("name %q err %v", name, err)
	}
	name, err = vlanIfaceName("enx001122334455", 100)
	if err != nil || name != "enx00112233.100" || len(name) > 15 {
		t.Fatalf("name %q err %v", name, err)
	}
	if _, err := vlanIfaceName("eth0", 0); err == nil {
		t.Fatal("vlan 0 should be rejected")
	}
	if _, err := vlanIfaceName("lo", 10); err == nil {
		t.Fatal("lo should be rejected")
	}
}

func TestParseVLANConfig(t *testing.T) {
	text := "VLAN Dev name\t | VLAN ID\nName-Type: VLAN_NAME_TYPE_RAW_PLUS_VID_NO_PAD\neth0.100       | 100  | eth0\n"
	got := parseVLANConfig(text)
	dev, ok := got["eth0.100"]
	if !ok || dev.id != 100 || dev.parent != "eth0" {
		t.Fatalf("%#v", got)
	}
}

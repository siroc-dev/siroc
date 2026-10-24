package sysops

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const cloudInitNet = `# This file is generated from information provided by the datasource.
network:
  version: 2
  ethernets:
    eth0:
      dhcp4: true
      addresses:
        - 192.168.1.10/24
      gateway4: 192.168.1.1
      nameservers:
        addresses:
          - 1.1.1.1
      routes:
        - to: 10.8.0.0/24
          via: 192.168.1.1
          metric: 50
`

func TestPlanNetplanAddKeepsExistingAddress(t *testing.T) {
	files := map[string]string{"50-cloud-init.yaml": cloudInitNet}
	plan, err := planNetplanAdd(files, "ethernets", "eth0", "eth0", 0, "10.0.0.5/24", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := plan.Update["50-cloud-init.yaml"]; ok {
		t.Fatal("address add should leave the existing file alone when no gateway is set")
	}
	got := plan.Update[sirocNetplanName]
	if !strings.Contains(got, "192.168.1.10/24") || !strings.Contains(got, "10.0.0.5/24") {
		t.Fatalf("missing addresses:\n%s", got)
	}
	if strings.Contains(got, "gateway4") || strings.Contains(got, "use-routes") {
		t.Fatalf("gateway changed:\n%s", got)
	}
	files[sirocNetplanName] = got
	plan, err = planNetplanAdd(files, "ethernets", "eth0", "eth0", 0, "10.0.0.6/24", "")
	if err != nil {
		t.Fatal(err)
	}
	got = plan.Update[sirocNetplanName]
	for _, want := range []string{"192.168.1.10/24", "10.0.0.5/24", "10.0.0.6/24"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s:\n%s", want, got)
		}
	}
}

func TestPlanNetplanAddGateway(t *testing.T) {
	files := map[string]string{"50-cloud-init.yaml": cloudInitNet}
	plan, err := planNetplanAdd(files, "ethernets", "eth0", "eth0", 0, "10.0.0.5/24", "10.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	base := plan.Update["50-cloud-init.yaml"]
	if strings.Contains(base, "gateway4") {
		t.Fatalf("old gateway kept:\n%s", base)
	}
	if !strings.Contains(base, "dhcp4: true") || !strings.Contains(base, "1.1.1.1") {
		t.Fatalf("base file lost other settings:\n%s", base)
	}
	got := plan.Update[sirocNetplanName]
	if !strings.Contains(got, "via: 10.0.0.1") || !strings.Contains(got, "use-routes: false") {
		t.Fatalf("gateway not saved:\n%s", got)
	}
	if !strings.Contains(got, "10.8.0.0/24") || !strings.Contains(got, "metric: 50") {
		t.Fatalf("other route dropped:\n%s", got)
	}
	overlay, err := parseOverlay(got)
	if err != nil {
		t.Fatal(err)
	}
	routes := overlay.Network.Ethernets["eth0"].Routes
	var defaults int
	for _, route := range routes {
		if route.To == "default" && route.Via != "10.0.0.1" {
			t.Fatalf("default via %s", route.Via)
		}
		if route.To == "default" {
			defaults++
		}
	}
	if defaults != 1 {
		t.Fatalf("defaults %d in %#v", defaults, routes)
	}
}

func TestPlanNetplanAddVLAN(t *testing.T) {
	plan, err := planNetplanAdd(map[string]string{}, "vlans", "eth0.100", "eth0", 100, "10.1.0.5/24", "10.1.0.1")
	if err != nil {
		t.Fatal(err)
	}
	got := plan.Update[sirocNetplanName]
	for _, want := range []string{"eth0.100:", "id: 100", "link: eth0", "10.1.0.5/24", "via: 10.1.0.1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s:\n%s", want, got)
		}
	}
}

func TestPlanNetplanAddRejects(t *testing.T) {
	files := map[string]string{"50-cloud-init.yaml": cloudInitNet}
	if _, err := planNetplanAdd(files, "ethernets", "eth0", "eth0", 0, "192.168.1.10/24", ""); err == nil {
		t.Fatal("duplicate address should fail")
	}
	if _, _, err := normalizeNetAddr("10.0.0.5/24", "2001:db8::1"); err == nil {
		t.Fatal("mixed IP versions should fail")
	}
	if _, _, err := normalizeNetAddr("10.0.0.5/24", "10.0.0.5"); err == nil {
		t.Fatal("gateway equal to the address should fail")
	}
	cidr, gw, err := normalizeNetAddr("10.0.0.5/24", "10.0.0.1")
	if err != nil || cidr != "10.0.0.5/24" || gw != "10.0.0.1" {
		t.Fatalf("cidr %s gw %s err %v", cidr, gw, err)
	}
}

func TestPlanNetplanDel(t *testing.T) {
	files := map[string]string{"50-cloud-init.yaml": cloudInitNet}
	added, err := planNetplanAdd(files, "ethernets", "eth0", "eth0", 0, "10.0.0.5/24", "")
	if err != nil {
		t.Fatal(err)
	}
	files[sirocNetplanName] = added.Update[sirocNetplanName]
	plan, err := planNetplanDel(files, "eth0", "10.0.0.5/24")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plan.Update[sirocNetplanName], "10.0.0.5/24") {
		t.Fatalf("address still in overlay:\n%s", plan.Update[sirocNetplanName])
	}
	if !strings.Contains(plan.Update[sirocNetplanName], "192.168.1.10/24") {
		t.Fatalf("original address dropped:\n%s", plan.Update[sirocNetplanName])
	}
	if _, ok := plan.Update["50-cloud-init.yaml"]; ok {
		t.Fatal("base file should stay unchanged when the address was only in the Siroc file")
	}
	baseOnly := map[string]string{"50-cloud-init.yaml": cloudInitNet}
	removed, err := planNetplanDel(baseOnly, "eth0", "192.168.1.10/24")
	if err != nil {
		t.Fatal(err)
	}
	base := removed.Update["50-cloud-init.yaml"]
	if strings.Contains(base, "192.168.1.10/24") || !strings.Contains(base, "dhcp4: true") || !strings.Contains(base, "1.1.1.1") {
		t.Fatalf("base address remove:\n%s", base)
	}

	vlan, err := planNetplanAdd(map[string]string{}, "vlans", "eth0.100", "eth0", 100, "10.1.0.5/24", "")
	if err != nil {
		t.Fatal(err)
	}
	vfiles := map[string]string{sirocNetplanName: vlan.Update[sirocNetplanName]}
	gone, err := planNetplanDel(vfiles, "eth0.100", "10.1.0.5/24")
	if err != nil {
		t.Fatal(err)
	}
	if len(gone.Delete) != 1 || gone.Delete[0] != sirocNetplanName {
		t.Fatalf("vlan file should be removed: %#v", gone)
	}
}

func TestStripGatewayKeepsNameservers(t *testing.T) {
	next, changed, err := stripGatewayKeys(cloudInitNet, "ethernets", "eth0")
	if err != nil || !changed {
		t.Fatalf("changed %v err %v", changed, err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(next), &doc); err != nil {
		t.Fatal(err)
	}
	eth := doc["network"].(map[string]any)["ethernets"].(map[string]any)["eth0"].(map[string]any)
	if _, ok := eth["gateway4"]; ok {
		t.Fatalf("gateway4 still present: %#v", eth)
	}
	if eth["dhcp4"] != true {
		t.Fatalf("dhcp4 = %#v", eth["dhcp4"])
	}
	ns := eth["nameservers"].(map[string]any)["addresses"].([]any)
	if len(ns) != 1 || ns[0] != "1.1.1.1" {
		t.Fatalf("nameservers %#v", ns)
	}
}

const bondNet = `
network:
  version: 2
  ethernets:
    ens22f0np0:
      match:
        macaddress: "9c:63:c0:1b:57:c6"
      set-name: "ens22f0np0"
    ens33f1np1:
      match:
        macaddress: "58:a2:e1:9a:ad:2f"
      set-name: "ens33f1np1"
  bonds:
    bond_public:
      addresses:
        - "2001:41d0:726:b600::/56"
      dhcp4: true
      accept-ra: false
      macaddress: "9c:63:c0:1b:57:c6"
      interfaces:
        - ens22f0np0
        - ens33f1np1
      parameters:
        mode: "802.3ad"
        lacp-rate: "fast"
        transmit-hash-policy: "layer3+4"
      routes:
        - on-link: true
          to: default
          via: "fe80::1"
`

func TestPlanNetplanAddBondKeepsType(t *testing.T) {
	files := map[string]string{
		"50-cloud-init.yaml": bondNet,
		sirocNetplanName: `
network:
  version: 2
  ethernets:
    bond_public:
      addresses:
        - 203.0.113.10/24
`,
	}
	plan, err := planNetplanAdd(files, "ethernets", "bond_public", "bond_public", 0, "203.0.113.20/24", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := plan.Update["50-cloud-init.yaml"]; ok {
		t.Fatal("bond file should stay unchanged")
	}
	got := plan.Update[sirocNetplanName]
	if strings.Contains(got, "ethernets:") {
		t.Fatalf("bond was written as an ethernet:\n%s", got)
	}
	if !strings.Contains(got, "bonds:") || !strings.Contains(got, "bond_public:") {
		t.Fatalf("bond section missing:\n%s", got)
	}
	for _, want := range []string{"2001:41d0:726:b600::/56", "203.0.113.10/24", "203.0.113.20/24"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s:\n%s", want, got)
		}
	}
	if strings.Contains(got, "interfaces:") || strings.Contains(got, "802.3ad") {
		t.Fatalf("overlay replaced bond members:\n%s", got)
	}

	withGW, err := planNetplanAdd(map[string]string{"50-cloud-init.yaml": bondNet}, "ethernets", "bond_public", "bond_public", 0, "203.0.113.20/24", "203.0.113.1")
	if err != nil {
		t.Fatal(err)
	}
	gw := withGW.Update[sirocNetplanName]
	if !strings.Contains(gw, "via: fe80::1") || !strings.Contains(gw, "on-link: true") || !strings.Contains(gw, "via: 203.0.113.1") {
		t.Fatalf("ipv6 default route was dropped:\n%s", gw)
	}
}

func TestOSReleaseID(t *testing.T) {
	if osReleaseID("NAME=Ubuntu\nID=ubuntu\n") != "ubuntu" {
		t.Fatal("ubuntu")
	}
	if osReleaseID("ID=\"debian\"\n") != "debian" {
		t.Fatal("debian")
	}
}

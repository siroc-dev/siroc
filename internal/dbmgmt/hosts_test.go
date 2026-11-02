package dbmgmt

import (
	"strings"
	"testing"
)

func TestResolveAccess(t *testing.T) {
	stored, hosts, err := ResolveAccess("", nil)
	if err != nil || stored != "" || strings.Join(hosts, ",") != "localhost,127.0.0.1" {
		t.Fatalf("local stored=%q hosts=%v err=%v", stored, hosts, err)
	}
	stored, hosts, err = ResolveAccess("all", nil)
	if err != nil || stored != "%" || strings.Join(hosts, ",") != "localhost,127.0.0.1,%" {
		t.Fatalf("all stored=%q hosts=%v err=%v", stored, hosts, err)
	}
	stored, hosts, err = ResolveAccess("ips", []string{"203.0.113.10", "198.51.100.8", "203.0.113.10"})
	if err != nil || stored != "198.51.100.8,203.0.113.10" {
		t.Fatalf("ips stored=%q err=%v", stored, err)
	}
	if strings.Join(hosts, ",") != "localhost,127.0.0.1,198.51.100.8,203.0.113.10" {
		t.Fatalf("ips hosts=%v", hosts)
	}
	if _, _, err := ResolveAccess("ips", nil); err == nil {
		t.Fatal("empty ip list should fail")
	}
	if _, _, err := ResolveAccess("ips", []string{"localhost"}); err == nil {
		t.Fatal("localhost should fail as a specific ip")
	}
	if _, _, err := ResolveAccess("ips", []string{"10.0.0.0/8"}); err == nil {
		t.Fatal("cidr should fail")
	}
}

func TestHostsToDropKeepsLocal(t *testing.T) {
	drop := HostsToDrop([]string{"localhost", "127.0.0.1", "%", "203.0.113.10"}, []string{"localhost", "127.0.0.1", "203.0.113.10"})
	if strings.Join(drop, ",") != "%" {
		t.Fatalf("%v", drop)
	}
	if HostsToDrop(LocalHosts(), LocalHosts()) != nil {
		t.Fatal("local to local should drop nothing")
	}
}

func TestDescribeStored(t *testing.T) {
	access, hosts := DescribeStored("")
	if access != AccessLocal || hosts != nil {
		t.Fatalf("%s %v", access, hosts)
	}
	access, hosts = DescribeStored("%")
	if access != AccessAll || hosts != nil {
		t.Fatalf("%s %v", access, hosts)
	}
	access, hosts = DescribeStored("198.51.100.8,203.0.113.10")
	if access != AccessIPs || strings.Join(hosts, ",") != "198.51.100.8,203.0.113.10" {
		t.Fatalf("%s %v", access, hosts)
	}
	if strings.Join(ExpandStored("%"), ",") != "localhost,127.0.0.1,%" {
		t.Fatalf("%v", ExpandStored("%"))
	}
}

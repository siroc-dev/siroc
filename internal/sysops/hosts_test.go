package sysops

import (
	"strings"
	"testing"
	"time"
)

const sampleHosts = "" +
	"127.0.0.1 localhost\n" +
	"127.0.1.1 web\n" +
	"\n" +
	"# The following lines are desirable for IPv6 capable hosts\n" +
	"::1 ip6-localhost ip6-loopback\n" +
	"\n" +
	"# BEGIN SIROC\n" +
	"127.0.0.1 scan.test\n" +
	"# END SIROC\n" +
	"10.0.0.5 db.internal # created 2026-10-01T01:02:03Z\n"

func TestListHostsSkipsScannerBlock(t *testing.T) {
	got := ListHostsText(sampleHosts)
	if len(got) != 4 {
		t.Fatalf("entries: %+v", got)
	}
	if got[0].IP != "127.0.0.1" || got[0].Names[0] != "localhost" || got[0].Created != "" {
		t.Fatalf("localhost: %+v", got[0])
	}
	if got[3].IP != "10.0.0.5" || got[3].Created != "2026-10-01T01:02:03Z" || got[3].Names[0] != "db.internal" {
		t.Fatalf("custom: %+v", got[3])
	}
	for _, e := range got {
		for _, n := range e.Names {
			if n == "scan.test" {
				t.Fatal("scanner name was listed")
			}
		}
	}
}

func TestMutateHostsAddKeepsCreatedAndScanner(t *testing.T) {
	when := time.Date(2026, 10, 15, 5, 48, 0, 0, time.UTC)
	next, entries, err := MutateHosts(sampleHosts, "add", 0, "10.1.2.3", []string{"cache.internal, backup.internal"}, when)
	if err != nil {
		t.Fatal(err)
	}
	last := entries[len(entries)-1]
	if last.IP != "10.1.2.3" || len(last.Names) != 2 || last.Created != "2026-10-15T05:48:00Z" {
		t.Fatalf("added: %+v", last)
	}
	if !strings.Contains(next, "# BEGIN SIROC\n127.0.0.1 scan.test\n# END SIROC\n") {
		t.Fatalf("scanner block changed:\n%s", next)
	}
	if !strings.Contains(next, "10.0.0.5\tdb.internal # created 2026-10-01T01:02:03Z") {
		t.Fatalf("existing created time lost:\n%s", next)
	}
	again, _, err := MutateHosts(next, "update", last.ID, "10.9.9.9", []string{"cache.internal"}, when.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(again, "10.9.9.9\tcache.internal # created 2026-10-15T05:48:00Z") {
		t.Fatalf("edit changed the created time:\n%s", again)
	}
	gone, entries, err := MutateHosts(again, "delete", last.ID, "", nil, when)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(gone, "cache.internal") {
		t.Fatalf("delete left the row:\n%s", gone)
	}
	if strings.Contains(gone, "scan.test") == false || len(entries) != 4 {
		t.Fatalf("delete touched other rows:\n%s", gone)
	}
}

func TestMutateHostsRejectsBadRows(t *testing.T) {
	if _, _, err := MutateHosts(sampleHosts, "add", 0, "nope", []string{"a.example"}, time.Now()); err == nil {
		t.Fatal("invalid ip accepted")
	}
	if _, _, err := MutateHosts(sampleHosts, "add", 0, "10.0.0.8", []string{"bad!host"}, time.Now()); err == nil {
		t.Fatal("invalid name accepted")
	}
	if _, _, err := MutateHosts(sampleHosts, "add", 0, "10.0.0.5", []string{"db.internal"}, time.Now()); err == nil {
		t.Fatal("duplicate accepted")
	}
	if _, _, err := MutateHosts(sampleHosts, "delete", 1, "", nil, time.Now()); err == nil {
		t.Fatal("localhost delete accepted")
	}
}

func TestManagedHostsSurviveRewrite(t *testing.T) {
	wiped := "" +
		"127.0.0.1 localhost\n" +
		"127.0.1.1 web\n" +
		"\n" +
		"# BEGIN SIROC\n" +
		"127.0.0.1 scan.test\n" +
		"# END SIROC\n"
	saved := managedHostText(sampleHosts)
	if !strings.Contains(saved, "10.0.0.5\tdb.internal # created 2026-10-01T01:02:03Z") || strings.Contains(saved, "localhost") {
		t.Fatalf("saved:\n%s", saved)
	}
	got, changed := mergeManagedHosts(wiped, saved)
	if !changed {
		t.Fatal("expected the saved row to be put back")
	}
	if !strings.Contains(got, "10.0.0.5\tdb.internal # created 2026-10-01T01:02:03Z") {
		t.Fatalf("row missing:\n%s", got)
	}
	if !strings.Contains(got, "localhost") || !strings.Contains(got, "127.0.1.1") || !strings.Contains(got, "scan.test") {
		t.Fatalf("system lines changed:\n%s", got)
	}
	again, changed := mergeManagedHosts(got, saved)
	if changed || again != got {
		t.Fatal("second pass should leave the file alone")
	}
	if unionManaged(saved, wiped) != saved {
		t.Fatal("a wiped hosts file should not drop the saved rows")
	}
	if !strings.Contains(unionManaged("", sampleHosts), "db.internal") {
		t.Fatal("current rows should be captured when nothing is saved yet")
	}
}

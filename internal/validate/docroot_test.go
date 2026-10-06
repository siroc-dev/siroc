package validate

import "testing"

func TestDocRootMountedDisk(t *testing.T) {
	got, err := DocRoot("/home", "alice", "/mnt/web/alice/public_html", "alice.test")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/mnt/web/alice/public_html" {
		t.Fatalf("%s", got)
	}
	if _, err := DocRoot("/home", "alice", "/mnt", "alice.test"); err == nil {
		t.Fatal("disk root should be rejected")
	}
	if _, err := DocRoot("/home", "alice", "/etc/nginx", "alice.test"); err == nil {
		t.Fatal("/etc should be rejected")
	}
	if _, err := DocRoot("/home", "alice", "/var/lib/siroc", "alice.test"); err == nil {
		t.Fatal("/var/lib should be rejected")
	}
	if _, err := DocRoot("/home", "alice", "/home/bob/public_html", "alice.test"); err == nil {
		t.Fatal("another account should be rejected")
	}
	if _, err := DocRoot("/home", "alice", "/home/alice/../../etc/passwd", "alice.test"); err == nil {
		t.Fatal("escape should be rejected")
	}
	inside, err := DocRoot("/home", "alice", "/home/alice/domains/alice.test/public_html", "alice.test")
	if err != nil || inside != "/home/alice/domains/alice.test/public_html" {
		t.Fatalf("%s %v", inside, err)
	}
	rel, err := DocRoot("/home", "alice", "domains/alice.test/public_html", "alice.test")
	if err != nil || rel == "" {
		t.Fatal(err)
	}
}

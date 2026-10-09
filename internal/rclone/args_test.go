package rclone

import "testing"

func TestCreateArgs(t *testing.T) {
	args, err := CreateArgs("offsite", "s3", map[string]string{
		"access_key_id":     "AKIA",
		"secret_access_key": "secret",
		"region":            "us-east-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := stringsJoin(args)
	want := "config create offsite s3 provider Other access_key_id AKIA secret_access_key secret region us-east-1"
	if got != want {
		t.Fatalf("got %q", got)
	}
	if _, err := CreateArgs("bad name", "s3", nil); err == nil {
		t.Fatal("name with a space should fail")
	}
	if _, err := CreateArgs("offsite", "drive", nil); err == nil {
		t.Fatal("unknown type should fail")
	}
}

func TestRunArgs(t *testing.T) {
	args, slow, err := RunArgs("sync", "/var/backups/siroc", "b2remote:bucket/siroc")
	if err != nil || !slow || stringsJoin(args) != "sync /var/backups/siroc b2remote:bucket/siroc" {
		t.Fatalf("args %v slow %v err %v", args, slow, err)
	}
	args, slow, err = RunArgs("lsd", "b2remote:", "")
	if err != nil || slow || stringsJoin(args) != "lsd b2remote:" {
		t.Fatalf("args %v slow %v err %v", args, slow, err)
	}
	if _, _, err := RunArgs("sync", "relative", "b2remote:"); err == nil {
		t.Fatal("relative source should fail")
	}
	if _, _, err := RunArgs("mount", "/tmp", "b2remote:"); err == nil {
		t.Fatal("mount is not a panel action")
	}
	if stringsJoin(ProgressFlags("copy")) != "--stats=1s --use-json-log" {
		t.Fatal("copy should report per-file stats")
	}
	if ProgressFlags("ls") != nil {
		t.Fatal("ls does not need transfer stats")
	}
}

func stringsJoin(args []string) string {
	out := ""
	for i, a := range args {
		if i > 0 {
			out += " "
		}
		out += a
	}
	return out
}

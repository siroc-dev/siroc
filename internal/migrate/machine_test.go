package migrate

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func TestMergeMachineKeepsSavedKey(t *testing.T) {
	prev := Machine{Host: "old", Password: "secret", PrivateKey: "KEY", KeyPassphrase: "phrase", KeyName: "desk.ppk"}
	got := MergeMachine(prev, rpc.DAConn{
		Mode: "ssh", Host: "203.0.113.10", Port: 22, Username: "ubuntu",
		UseSavedKey: true, UseSavedPassword: true, UseSavedPassphrase: true, KeyName: "desk.ppk",
	})
	if got.Host != "203.0.113.10" || got.Password != "secret" || got.PrivateKey != "KEY" || got.KeyPassphrase != "phrase" || got.KeyName != "desk.ppk" {
		t.Fatalf("%#v", got)
	}
}

func TestMergeMachineReplacesAndClearsKey(t *testing.T) {
	prev := Machine{PrivateKey: "OLD", KeyName: "old.ppk", Password: "pw"}
	got := MergeMachine(prev, rpc.DAConn{Mode: "ssh", Host: "h", Username: "root", PrivateKey: "NEW", KeyName: "new.ppk", Password: "next"})
	if got.PrivateKey != "NEW" || got.KeyName != "new.ppk" || got.Password != "next" {
		t.Fatalf("%#v", got)
	}
	cleared := MergeMachine(got, rpc.DAConn{Mode: "ssh", Host: "h", Username: "root", UseSavedPassword: true})
	if cleared.PrivateKey != "" || cleared.KeyName != "" || cleared.Password != "next" {
		t.Fatalf("%#v", cleared)
	}
}

func TestMachineFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "migrate-machine.json")
	want := Machine{Mode: "ssh", Host: "203.0.113.10", Port: 22, Username: "ubuntu", KeyName: "desk.ppk", PrivateKey: "enc-key"}
	if err := SaveMachine(path, want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("mode %o", info.Mode().Perm())
	}
	got, err := LoadMachine(path)
	if err != nil || got.PrivateKey != "enc-key" || got.Host != want.Host {
		t.Fatalf("%#v %v", got, err)
	}
	missing, err := LoadMachine(filepath.Join(dir, "missing.json"))
	if err != nil || missing.Host != "" {
		t.Fatalf("%#v %v", missing, err)
	}
}

package sysops

import (
	"strings"
	"testing"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func TestExportsRoundTrip(t *testing.T) {
	in := "" +
		"# kept\n" +
		"/srv/share 10.0.0.0/24(rw,sync,no_subtree_check,root_squash) 10.0.0.5(ro,all_squash,anonuid=65534,anongid=65534,fsid=0)\n" +
		"/srv/share \\\n" +
		"  10.0.0.9(rw,insecure)\n"
	comments, entries := splitExports(in)
	if len(comments) != 1 || comments[0] != "# kept" {
		t.Fatalf("comments: %#v", comments)
	}
	if len(entries) != 3 {
		t.Fatalf("entries: %+v", entries)
	}
	if entries[1].anonUID != "65534" || entries[1].fsid != "0" || entries[1].flags[0] != "ro" {
		t.Fatalf("second: %+v", entries[1])
	}
	if entries[2].client != "10.0.0.9" || !contains(entries[2].flags, "insecure") {
		t.Fatalf("continued: %+v", entries[2])
	}
	out := renderExports(comments, entries)
	if !strings.Contains(out, "# kept\n") || !strings.Contains(out, "/srv/share 10.0.0.0/24(rw,sync,no_subtree_check,root_squash) 10.0.0.5(ro,all_squash,anonuid=65534,anongid=65534,fsid=0)") {
		t.Fatalf("render:\n%s", out)
	}
	next, err := MutateExports(in, "add", 0, rpc.NFSExportReq{Path: "/mnt/data", Client: "*", Options: []string{"async", "rw", "no_root_squash"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(next, "/mnt/data *(rw,async,no_root_squash)") {
		t.Fatalf("add order:\n%s", next)
	}
	if _, err := MutateExports(next, "add", 0, rpc.NFSExportReq{Path: "/etc/passwd", Client: "*", Options: []string{"ro"}}); err == nil {
		t.Fatal("system path accepted")
	}
	if _, err := MutateExports(next, "add", 0, rpc.NFSExportReq{Path: "/mnt/data", Client: "*", Options: []string{"rw", "ro"}}); err == nil {
		t.Fatal("both access modes accepted")
	}
	gone, err := MutateExports(next, "delete", 1, rpc.NFSExportReq{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(gone, "10.0.0.0/24") {
		t.Fatalf("delete left the client:\n%s", gone)
	}
	if !strings.Contains(gone, "10.0.0.5") || !strings.Contains(gone, "# kept") {
		t.Fatalf("delete touched other rows:\n%s", gone)
	}
}

func TestMountOptionsAndFstab(t *testing.T) {
	opts, err := MountOptionString(rpc.NFSMountReq{
		Source: "10.0.0.5:/srv/share", MountPoint: "/mnt/share",
		Options: []string{"_netdev", "hard"}, NFSVers: "4.2", Proto: "tcp", Timeo: 600,
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts != "hard,_netdev,nfsvers=4.2,proto=tcp,timeo=600" {
		t.Fatal(opts)
	}
	src, _, err := cleanMountReq(rpc.NFSMountReq{Source: "[2001:db8::5]:/srv/share", MountPoint: "/mnt/share"})
	if err != nil || src != "[2001:db8::5]:/srv/share" {
		t.Fatalf("ipv6 source: %s %v", src, err)
	}
	fstab := "/dev/sda1 / ext4 defaults 0 1\n10.0.0.5:/srv/share /mnt/share nfs hard,_netdev 0 0\n"
	proc := "10.0.0.5:/srv/share /mnt/share nfs4 rw,vers=4.2,proto=tcp 0 0\n10.0.0.8:/other /mnt/other nfs rw,soft 0 0\n"
	mounts := mergeNFSMounts(fstab, proc)
	if len(mounts) != 2 || !mounts[0].Persisted || !mounts[0].Mounted || mounts[0].NFSVers != "4.2" {
		t.Fatalf("merged: %+v", mounts)
	}
	if mounts[1].Persisted || !mounts[1].Mounted || mounts[1].MountPoint != "/mnt/other" {
		t.Fatalf("live only: %+v", mounts[1])
	}
	next, err := MutateFstabNFS(fstab, "add", "/mnt/backup", "10.0.0.9:/backup", "soft,_netdev")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(next, "/dev/sda1 / ext4 defaults 0 1") || !strings.Contains(next, "10.0.0.9:/backup /mnt/backup nfs soft,_netdev 0 0") {
		t.Fatalf("fstab:\n%s", next)
	}
	updated, err := MutateFstabNFS(next, "add", "/mnt/share", "10.0.0.5:/srv/share", "ro,hard")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(updated, "/mnt/share") != 1 || !strings.Contains(updated, "ro,hard") {
		t.Fatalf("replace:\n%s", updated)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

package sysops

import (
	"fmt"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func DiskMountOptions() []rpc.NFSOption {
	return []rpc.NFSOption{
		{Key: "rw", Group: "access", Exclusive: true, Label: "Read and write", Detail: "Files on this disk can be changed."},
		{Key: "ro", Group: "access", Exclusive: true, Label: "Read only", Detail: "Files can be read but not changed. Useful for a disk you do not want to modify."},
		{Key: "relatime", Group: "atime", Exclusive: true, Label: "Relative access time", Detail: "Update a file's access time only when it is older than the modified time. This is the usual Linux default."},
		{Key: "noatime", Group: "atime", Exclusive: true, Label: "No access time", Detail: "Do not update access times. Fewer writes, which helps disks and SSDs."},
		{Key: "strictatime", Group: "atime", Exclusive: true, Label: "Strict access time", Detail: "Update the access time on every read. More writes, and some older programs expect it."},
		{Key: "nodiratime", Group: "dirtime", Label: "No directory access time", Detail: "Do not update access times on directories. This can be combined with the file access-time choice."},
		{Key: "nosuid", Group: "safety", Label: "Ignore setuid", Detail: "Ignore setuid and setgid bits, so a program on this disk cannot run as another user."},
		{Key: "nodev", Group: "safety", Label: "Ignore devices", Detail: "Ignore device files on this disk."},
		{Key: "noexec", Group: "safety", Label: "No execute", Detail: "Do not run programs stored on this disk."},
		{Key: "discard", Group: "ssd", Label: "Trim discarded blocks", Detail: "Tell an SSD which blocks are free. Use this on solid-state disks, not on a spinning disk."},
		{Key: "nofail", Group: "boot", Label: "Boot if missing", Detail: "Continue booting when this disk is not attached. Use this for USB and extra data disks."},
		{Key: "errors=remount-ro", Group: "errors", Label: "Read-only on error", Detail: "If the filesystem reports an error, remount it read-only instead of keeping it writable."},
		{Key: "user", Group: "owner", Exclusive: true, Label: "Allow a user to mount", Detail: "A logged-in user may mount and unmount this disk. It also implies noexec, nosuid, and nodev unless you set otherwise."},
		{Key: "nouser", Group: "owner", Exclusive: true, Label: "Root only", Detail: "Only root may mount this disk."},
	}
}

func DiskFilesystems() []rpc.NFSChoice {
	return []rpc.NFSChoice{
		{Value: "", Label: "Detect", Detail: "Let mount recognize the filesystem. Use this when you are not sure."},
		{Value: "ext4", Label: "ext4", Detail: "The usual Linux filesystem. Journaled, and a good fit for most disks."},
		{Value: "xfs", Label: "XFS", Detail: "Strong with large files and many cores. It cannot be shrunk later."},
		{Value: "btrfs", Label: "Btrfs", Detail: "Checksums and snapshots. Leave free space; a nearly full Btrfs disk is hard to recover."},
		{Value: "vfat", Label: "FAT32", Detail: "USB sticks and EFI partitions. No Unix owners or permissions, and files must be under 4 GB."},
		{Value: "exfat", Label: "exFAT", Detail: "Large files on a drive shared with Windows and macOS."},
		{Value: "ntfs3", Label: "NTFS", Detail: "A Windows NTFS disk, using the kernel driver."},
		{Value: "swap", Label: "Swap", Detail: "Extra swap space, not a directory of files. It is mounted with the sw option."},
	}
}

func CleanDiskOptions(fs, raw string) (string, string, error) {
	fs = strings.TrimSpace(strings.ToLower(fs))
	if fs == "ntfs" {
		fs = "ntfs3"
	}
	known := false
	for _, item := range DiskFilesystems() {
		if item.Value == fs {
			known = true
			break
		}
	}
	if !known {
		return "", "", fmt.Errorf("unknown filesystem")
	}
	if fs == "swap" {
		return "swap", "sw", nil
	}
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "defaults" {
		return fs, "defaults", nil
	}
	flags, err := cleanFlags(strings.Split(raw, ","), DiskMountOptions())
	if err != nil {
		return "", "", err
	}
	if len(flags) == 0 {
		return fs, "defaults", nil
	}
	return fs, strings.Join(flags, ","), nil
}

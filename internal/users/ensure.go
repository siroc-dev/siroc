package users

import "strconv"

// EnsureUseraddArgs builds useradd flags that keep an existing home directory
// (typical after a container rebuild) instead of failing or allocating a new UID.
func EnsureUseraddArgs(username, home string, uid, gid int, homeExists bool) []string {
	args := make([]string, 0, 10)
	if homeExists {
		args = append(args, "-M")
	} else {
		args = append(args, "-m")
	}
	args = append(args, "-d", home, "-s", "/bin/bash")
	if uid > 0 {
		args = append(args, "-u", strconv.Itoa(uid))
	}
	if gid > 0 {
		args = append(args, "-g", strconv.Itoa(gid))
	}
	return append(args, username)
}

func EnsureGroupaddArgs(name string, gid int) []string {
	if gid > 0 {
		return []string{"-g", strconv.Itoa(gid), name}
	}
	return []string{name}
}

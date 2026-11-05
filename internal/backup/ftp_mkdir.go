package backup

import "strings"

// ftpMkdirQuotes builds curl --quote commands that create each missing directory.
// A leading * tells curl to continue when the directory already exists.
func ftpMkdirQuotes(remoteFile string) []string {
	remoteFile = strings.TrimSpace(strings.ReplaceAll(remoteFile, "\\", "/"))
	if remoteFile == "" || strings.ContainsAny(remoteFile, "\r\n\"") {
		return nil
	}
	slash := strings.LastIndex(remoteFile, "/")
	if slash <= 0 {
		return nil
	}
	dir := remoteFile[:slash]
	var parts []string
	for _, part := range strings.Split(strings.Trim(dir, "/"), "/") {
		if part == "" || part == "." || part == ".." {
			return nil
		}
		parts = append(parts, part)
	}
	var args []string
	cur := ""
	for _, part := range parts {
		cur += "/" + part
		args = append(args, "--quote", "*MKD "+cur)
	}
	return args
}

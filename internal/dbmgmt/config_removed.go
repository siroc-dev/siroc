package dbmgmt

import "strings"

// removedMariaDBOptions are settings MariaDB 10.6 and later reject while
// reading option files. innodb_buffer_pool_instances was removed in 10.6;
// leaving it in a cnf makes mariadbd exit with
// "Fatal error in defaults handling. Program aborted".
var removedMariaDBOptions = map[string]bool{
	"innodb_buffer_pool_instances": true,
	"innodb_page_cleaners":         true,
}

func removedMariaDBOption(name string) bool {
	name = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), "-", "_"))
	return removedMariaDBOptions[name]
}

// stripRemovedMariaDBOptions drops option lines MariaDB 10.6+ cannot start
// with. Comments and every other setting stay put.
func stripRemovedMariaDBOptions(body string) (string, bool) {
	lines := strings.Split(body, "\n")
	out := make([]string, 0, len(lines))
	changed := false
	for _, line := range lines {
		if removedMariaDBOption(mysqlOptionName(line)) {
			changed = true
			continue
		}
		out = append(out, line)
	}
	if !changed {
		return body, false
	}
	return strings.Join(out, "\n"), true
}

// stripMySQLOption removes one option, matching either underscore or hyphen form.
func stripMySQLOption(body, name string) (string, bool) {
	want := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), "-", "_"))
	if want == "" {
		return body, false
	}
	lines := strings.Split(body, "\n")
	out := make([]string, 0, len(lines))
	changed := false
	for _, line := range lines {
		got := strings.ToLower(strings.ReplaceAll(mysqlOptionName(line), "-", "_"))
		if got == want {
			changed = true
			continue
		}
		out = append(out, line)
	}
	if !changed {
		return body, false
	}
	return strings.Join(out, "\n"), true
}

// missingDefaultsDir is the directory MariaDB could not read. The message is
// "Can't read dir of '/etc/mysql/conf.d/' (Errcode: 2 ...)". Journald often
// hides that line as blob data and only shows the following fatal line.
func missingDefaultsDir(msg string) string {
	const mark = "Can't read dir of '"
	i := strings.Index(msg, mark)
	if i < 0 {
		return ""
	}
	rest := msg[i+len(mark):]
	j := strings.Index(rest, "'")
	if j <= 0 {
		return ""
	}
	dir := rest[:j]
	if strings.Contains(dir, "..") || !strings.HasPrefix(dir, "/etc/mysql/") {
		return ""
	}
	return strings.TrimRight(dir, "/")
}

// unknownDefaultsOption is the option name from "unknown variable 'name=value'".
func unknownDefaultsOption(msg string) string {
	const mark = "unknown variable '"
	i := strings.Index(msg, mark)
	if i < 0 {
		return ""
	}
	rest := msg[i+len(mark):]
	j := strings.Index(rest, "'")
	if j <= 0 {
		return ""
	}
	name := rest[:j]
	if eq := strings.IndexAny(name, "= "); eq >= 0 {
		name = name[:eq]
	}
	name = strings.TrimSpace(strings.TrimPrefix(name, "--"))
	if name == "" || strings.ContainsAny(name, "\\/\n\r") {
		return ""
	}
	return name
}

func mysqlOptionName(line string) string {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "[") {
		return ""
	}
	name := line
	if i := strings.IndexAny(line, " ="); i >= 0 {
		name = line[:i]
	}
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "--")
	return name
}

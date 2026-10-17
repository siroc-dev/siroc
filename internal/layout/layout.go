package layout

import "strings"

const (
	Root  = "/siroc"
	Bin   = "/siroc/bin"
	Web   = "/siroc/web"
	Data  = "/siroc/data"
	Log   = "/siroc/log"
	Cache = "/siroc/cache"
	DB    = "/siroc/db/mysql"
	Etc   = "/siroc/etc"
)

// RewriteUnit points a systemd unit at /siroc. Other lines stay as written.
func RewriteUnit(text string) (string, bool) {
	lines := strings.Split(text, "\n")
	changed := false
	for i, line := range lines {
		next := rewriteUnitLine(line)
		if next != line {
			lines[i] = next
			changed = true
		}
	}
	if !changed {
		return text, false
	}
	return strings.Join(lines, "\n"), true
}

func rewriteUnitLine(line string) string {
	trim := strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(trim, "ExecStart="):
		line = strings.ReplaceAll(line, "/usr/local/bin/siroc-agent", Bin+"/siroc-agent")
		line = strings.ReplaceAll(line, "/usr/local/bin/siroc-panel", Bin+"/siroc-panel")
		line = strings.ReplaceAll(line, "/opt/siroc/bin/siroc-agent", Bin+"/siroc-agent")
		line = strings.ReplaceAll(line, "/opt/siroc/bin/siroc-panel", Bin+"/siroc-panel")
	case strings.Contains(trim, "SIROC_ROOT="):
		line = replaceEnv(line, "SIROC_ROOT", "/opt/siroc", Root)
	case strings.Contains(trim, "SIROC_DATA_DIR="):
		line = replaceEnv(line, "SIROC_DATA_DIR", "/var/lib/siroc", Data)
	case strings.Contains(trim, "SIROC_WEB_DIR="):
		line = replaceEnv(line, "SIROC_WEB_DIR", "/opt/siroc/web/dist", Web+"/dist")
	}
	return line
}

func replaceEnv(line, key, old, next string) string {
	for _, prefix := range []string{key + "=", key + "=\""} {
		if i := strings.Index(line, prefix); i >= 0 {
			start := i + len(prefix)
			if strings.HasPrefix(line[start:], old) {
				rest := line[start+len(old):]
				if rest == "" || rest[0] == '"' || rest[0] == ' ' {
					return line[:start] + next + rest
				}
			}
		}
	}
	return line
}

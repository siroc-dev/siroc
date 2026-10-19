package hosting

import "strings"

// Makes the local npm shims executable. A fresh install, or the web permission
// walk, can leave vite or vp as a file or symlink without the execute bit.
// npm then runs the package script with sh and reports "vp: Permission denied".
const npmBinFix = `if [ -d node_modules/.bin ]; then find node_modules/.bin \( -type f -o -type l \) -exec chmod a+x {} + 2>/dev/null || true; find node_modules/.bin -type l -exec sh -c 'for l; do t=$(readlink -f "$l" 2>/dev/null) || continue; [ -f "$t" ] && chmod a+x "$t"; done' _ {} + 2>/dev/null || true; fi`

func wrapNpmBins(command string) string {
	var b strings.Builder
	b.WriteString(npmBinFix)
	b.WriteByte('\n')
	for _, line := range strings.Split(command, "\n") {
		b.WriteString(line)
		b.WriteByte('\n')
		if npmInstalls(line) {
			b.WriteString(npmBinFix)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func npmInstalls(line string) bool {
	fields := strings.Fields(line)
	if len(fields) < 2 || !npmCommand(fields[0]) {
		return false
	}
	switch fields[1] {
	case "install", "i", "ci", "add":
		return true
	default:
		return false
	}
}

func npmCommand(field string) bool {
	field = strings.Trim(field, `"'`)
	return field == "npm" || strings.HasSuffix(field, "/npm")
}

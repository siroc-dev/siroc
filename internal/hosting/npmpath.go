package hosting

import "strings"

// Makes the local npm shims executable. A fresh install can leave vite as a
// symlink to a file without the execute bit, which fails as "vite: Permission denied".
const npmBinFix = `if [ -d node_modules/.bin ]; then find node_modules/.bin \( -type f -o -type l \) -exec chmod a+x {} + 2>/dev/null || true; fi`

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
	if len(fields) < 2 || fields[0] != "npm" {
		return false
	}
	switch fields[1] {
	case "install", "i", "ci", "add":
		return true
	default:
		return false
	}
}

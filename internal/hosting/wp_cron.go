package hosting

import (
	"regexp"
	"strings"
)

var wpDisableCronRe = regexp.MustCompile(`(?m)^[ \t]*define\s*\(\s*['"]DISABLE_WP_CRON['"]\s*,\s*(true|false|1|0)\s*\)\s*;`)

func wpCronDisabled(src string) bool {
	m := wpDisableCronRe.FindStringSubmatch(src)
	if len(m) < 2 {
		return false
	}
	switch strings.ToLower(m[1]) {
	case "true", "1":
		return true
	default:
		return false
	}
}

func wpCronState(system bool, config string) (on, systemOn bool) {
	if system {
		return true, true
	}
	return !wpCronDisabled(config), false
}

func setWPCronDisabled(src string, disabled bool) string {
	val := "false"
	if disabled {
		val = "true"
	}
	line := "define( 'DISABLE_WP_CRON', " + val + " );"
	if wpDisableCronRe.MatchString(src) {
		return wpDisableCronRe.ReplaceAllString(src, line)
	}
	if !disabled {
		return src
	}
	const stop = "/* That's all, stop editing!"
	if i := strings.Index(src, stop); i >= 0 {
		return src[:i] + line + "\n" + src[i:]
	}
	if i := strings.Index(src, "wp-settings.php"); i >= 0 {
		lineStart := strings.LastIndex(src[:i], "\n") + 1
		return src[:lineStart] + line + "\n" + src[lineStart:]
	}
	if src != "" && !strings.HasSuffix(src, "\n") {
		src += "\n"
	}
	return src + line + "\n"
}

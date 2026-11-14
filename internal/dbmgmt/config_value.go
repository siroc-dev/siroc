package dbmgmt

import (
	"regexp"
	"strings"
)

var (
	dbTZOffsetRe = regexp.MustCompile(`^[+-](?:0[0-9]|1[0-4]):[0-5][0-9](?::[0-5][0-9])?$`)
	dbTZNameRe   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_+-]*(?:/[A-Za-z0-9_+-]+)+$`)
)

func dbShownValue(live, fileVal, recommend string, preset bool) string {
	if preset && recommend != "" {
		return recommend
	}
	if live != "" {
		return live
	}
	if fileVal != "" {
		return fileVal
	}
	return recommend
}

func unquoteMySQL(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 {
		if (v[0] == '\'' && v[len(v)-1] == '\'') || (v[0] == '"' && v[len(v)-1] == '"') {
			v = strings.TrimSpace(v[1 : len(v)-1])
		}
	}
	return v
}

func normalizeDBTimezone(v string) (string, bool) {
	v = unquoteMySQL(v)
	if v == "" || len(v) > 64 || strings.ContainsAny(v, " \t\r\n;`'\"\\") {
		return "", false
	}
	if strings.EqualFold(v, "SYSTEM") {
		return "SYSTEM", true
	}
	if dbTZOffsetRe.MatchString(v) {
		return v, true
	}
	if v == "UTC" || v == "GMT" || dbTZNameRe.MatchString(v) {
		return v, true
	}
	return "", false
}

func dbTimezoneNeedsTables(v string) bool {
	norm, ok := normalizeDBTimezone(v)
	if !ok || norm == "SYSTEM" || strings.HasPrefix(norm, "+") || strings.HasPrefix(norm, "-") {
		return false
	}
	return true
}

func mysqlTimezoneConf(v string) string {
	norm, ok := normalizeDBTimezone(v)
	if !ok || norm == "SYSTEM" {
		return "SYSTEM"
	}
	return "'" + norm + "'"
}

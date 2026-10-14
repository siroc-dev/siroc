package hosting

import (
	"fmt"
	"regexp"
	"strings"
)

func nginxAccessLine(domain string, on bool) string {
	if !on {
		return "    access_log off;"
	}
	return fmt.Sprintf("    access_log /var/log/nginx/sites/%s-access.log combined;", domain)
}

func nginxErrorLine(domain string, on bool) string {
	if !on {
		return "    error_log /dev/null emerg;"
	}
	return fmt.Sprintf("    error_log /var/log/nginx/sites/%s-error.log;", domain)
}

func apacheErrorLine(domain string, on bool) string {
	if !on {
		return "    ErrorLog /dev/null"
	}
	return fmt.Sprintf("    ErrorLog ${APACHE_LOG_DIR}/sites/%s-error.log", domain)
}

func apacheAccessLine(domain string, on bool) string {
	if !on {
		return "    CustomLog /dev/null combined"
	}
	return fmt.Sprintf("    CustomLog ${APACHE_LOG_DIR}/sites/%s-access.log combined", domain)
}

var (
	nginxAccessRe  = regexp.MustCompile(`(?m)^[ \t]*access_log\s+[^;\n]*;`)
	nginxErrorRe   = regexp.MustCompile(`(?m)^[ \t]*error_log\s+[^;\n]*;`)
	apacheErrorRe  = regexp.MustCompile(`(?m)^[ \t]*ErrorLog\s+.*$`)
	apacheAccessRe = regexp.MustCompile(`(?m)^[ \t]*CustomLog\s+.*$`)
)

func applyNginxLogLines(body, access, errLog string) string {
	body = replaceOrInsert(body, nginxAccessRe, access, "client_max_body_size", "server_name")
	return replaceOrInsert(body, nginxErrorRe, errLog, "access_log", "client_max_body_size")
}

func applyApacheLogLines(body, errLog, access string) string {
	body = replaceOrInsert(body, apacheErrorRe, errLog, "DocumentRoot", "ServerName")
	return replaceOrInsert(body, apacheAccessRe, access, "ErrorLog", "DocumentRoot")
}

func replaceOrInsert(body string, re *regexp.Regexp, line, anchor, fallback string) string {
	if strings.TrimSpace(line) == "" {
		return body
	}
	if re.MatchString(body) {
		return re.ReplaceAllString(body, line)
	}
	if next, ok := insertAfter(body, anchor, line); ok {
		return next
	}
	if next, ok := insertAfter(body, fallback, line); ok {
		return next
	}
	return line + "\n" + body
}

func insertAfter(body, anchor, line string) (string, bool) {
	if anchor == "" || !strings.Contains(body, anchor) {
		return body, false
	}
	parts := strings.Split(body, "\n")
	for i, row := range parts {
		if strings.Contains(row, anchor) {
			parts = append(parts[:i+1], append([]string{line}, parts[i+1:]...)...)
			return strings.Join(parts, "\n"), true
		}
	}
	return body, false
}

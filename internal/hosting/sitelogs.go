package hosting

import (
	"fmt"
	"regexp"
	"strings"
)

const assetLogMap = "map $uri $siroc_asset_log {\n    default 1;\n    ~*\\.(?:css|js|mjs|gif|jpe?g|png|bmp|ico|svg|webp|avif|mp4|m4v|webm|mkv|avi|mov|wmv|flv|mpeg|mpg)$ 0;\n}\n"

func nginxAccessLine(domain string, on bool) string {
	if !on {
		return "    access_log off;"
	}
	return fmt.Sprintf("    access_log /var/log/nginx/sites/%s-access.log combined if=$siroc_asset_log;", domain)
}

// quietAssetAccessLogs stops access-log lines for images, video, CSS, and JavaScript.
func quietAssetAccessLogs(body string) string {
	return assetAccessRe.ReplaceAllString(body, "${1} if=$$siroc_asset_log;")
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
	assetAccessRe  = regexp.MustCompile(`(?m)^([ \t]*access_log\s+/var/log/nginx/sites/\S+\s+combined)(?:\s+if=\$siroc_asset_log)?;`)
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

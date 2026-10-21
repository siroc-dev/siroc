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

func withoutHSTS(body string) string {
	var kept []string
	for _, line := range strings.Split(body, "\n") {
		if setsHSTS(line) {
			continue
		}
		kept = append(kept, line)
	}
	body = strings.Join(kept, "\n")
	if strings.Contains(body, "<VirtualHost") {
		return ensureApacheUnsetHSTS(body)
	}
	return ensureNginxHideHSTS(body)
}

func setsHSTS(line string) bool {
	s := strings.ToLower(strings.TrimSpace(line))
	if !strings.Contains(s, "strict-transport-security") {
		return false
	}
	return !strings.Contains(s, "proxy_hide_header") && !strings.Contains(s, "unset")
}

func ensureNginxHideHSTS(body string) string {
	lines := strings.Split(body, "\n")
	type span struct{ open, end int }
	var blocks []span
	depth := 0
	serverAt := -1
	serverDepth := 0
	for i, line := range lines {
		if serverAt < 0 && strings.HasPrefix(strings.TrimSpace(line), "server {") {
			serverAt = i
			serverDepth = depth
		}
		depth += strings.Count(line, "{") - strings.Count(line, "}")
		if serverAt >= 0 && depth <= serverDepth {
			blocks = append(blocks, span{serverAt, i})
			serverAt = -1
		}
	}
	const hide = "    proxy_hide_header Strict-Transport-Security;"
	for i := len(blocks) - 1; i >= 0; i-- {
		b := blocks[i]
		chunk := strings.ToLower(strings.Join(lines[b.open:b.end+1], "\n"))
		if strings.Contains(chunk, "proxy_hide_header strict-transport-security") {
			continue
		}
		lines = append(lines[:b.open+1], append([]string{hide}, lines[b.open+1:]...)...)
	}
	return strings.Join(lines, "\n")
}

func ensureApacheUnsetHSTS(body string) string {
	if strings.Contains(strings.ToLower(body), "unset strict-transport-security") {
		return body
	}
	block := "    <IfModule mod_headers.c>\n        Header always unset Strict-Transport-Security\n    </IfModule>"
	anchor := "DocumentRoot"
	if !strings.Contains(body, anchor) {
		anchor = "<VirtualHost"
	}
	var out []string
	for _, line := range strings.Split(body, "\n") {
		out = append(out, line)
		if strings.Contains(line, anchor) {
			out = append(out, block)
		}
	}
	return strings.Join(out, "\n")
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

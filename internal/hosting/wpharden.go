//go:build linux

package hosting

import (
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/validate"
)

const wpHardenPlugin = `<?php
/**
 * Plugin Name: Siroc Harden
 */
if (!defined('ABSPATH')) {
    return;
}
$cp_harden = array('xmlrpc' => false, 'pingbacks' => false, 'uploadsphp' => false);
$file = __DIR__ . '/siroc-harden-settings.php';
if (is_readable($file)) {
    $loaded = include $file;
    if (is_array($loaded)) {
        $cp_harden = array_merge($cp_harden, $loaded);
    }
}
if (!empty($cp_harden['uploadsphp'])) {
    add_filter('upload_mimes', function ($mimes) {
        foreach (array_keys((array) $mimes) as $ext) {
            if (preg_match('/php|phtml|phar|pht/i', (string) $ext)) {
                unset($mimes[$ext]);
            }
        }
        return $mimes;
    }, 99);
    add_filter('wp_handle_upload_prefilter', function ($file) {
        $name = strtolower((string) ($file['name'] ?? ''));
        if (preg_match('/\\.(?:php[0-9]?|phtml|phar|pht)(?:\\.|$)/', $name)) {
            $file['error'] = 'PHP files are not allowed in the uploads folder.';
        }
        return $file;
    }, 99);
}
if (!empty($cp_harden['xmlrpc'])) {
    add_filter('xmlrpc_enabled', '__return_false', 99);
    add_action('wp', function () {
        if (defined('XMLRPC_REQUEST') && XMLRPC_REQUEST) {
            status_header(403);
            exit;
        }
    }, 0);
}
if (!empty($cp_harden['pingbacks'])) {
    add_filter('xmlrpc_methods', function ($methods) {
        unset($methods['pingback.ping'], $methods['pingback.extensions.getPingbacks']);
        return $methods;
    }, 99);
    add_filter('wp_headers', function ($headers) {
        unset($headers['X-Pingback']);
        return $headers;
    }, 99);
    add_filter('pings_open', '__return_false', 99);
    add_filter('pre_option_default_ping_status', function () {
        return 'closed';
    });
    add_filter('pre_option_default_pingback_flag', function () {
        return '0';
    });
}
`

var (
	wpLoginRe     = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._-]{2,31}$`)
	wpNicenameRe  = regexp.MustCompile(`[^a-z0-9-]+`)
	wpPostIDRe    = regexp.MustCompile(`^[0-9]{1,12}$`)
	wpReservedLog = map[string]struct{}{"administrator": {}, "root": {}, "www-data": {}}
)

type wpHarden struct {
	XMLRPC     bool
	Pingbacks  bool
	UploadsPHP bool
}

func randomWPLogin() string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	const alnum = letters + "0123456789"
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("u%06x", time.Now().UnixNano()%0xffffff)
	}
	out := make([]byte, 8)
	out[0] = letters[int(b[0])%len(letters)]
	for i := 1; i < 8; i++ {
		out[i] = alnum[int(b[i])%len(alnum)]
	}
	return string(out)
}

func validWPLogin(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return randomWPLogin(), nil
	}
	if !wpLoginRe.MatchString(p) {
		return "", fmt.Errorf("username must start with a letter and use A-Z 0-9 . _ - (3-32 chars)")
	}
	if _, bad := wpReservedLog[strings.ToLower(p)]; bad {
		return "", fmt.Errorf("username %s is reserved", p)
	}
	return p, nil
}

func wpNicename(login string) string {
	s := strings.ToLower(strings.TrimSpace(login))
	s = wpNicenameRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "user"
	}
	if len(s) > 50 {
		s = s[:50]
	}
	return s
}

func wpHardenDir(root string) string {
	return filepath.Join(root, "wp-content", "mu-plugins")
}

func wpHardenSettingsPath(root string) string {
	return filepath.Join(wpHardenDir(root), "siroc-harden-settings.php")
}

func phpBool(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func readWPHarden(root string) wpHarden {
	b, err := os.ReadFile(wpHardenSettingsPath(root))
	if err != nil {
		return wpHarden{}
	}
	s := string(b)
	return wpHarden{
		XMLRPC:     strings.Contains(s, "'xmlrpc'=>true") || strings.Contains(s, `"xmlrpc"=>true`),
		Pingbacks:  strings.Contains(s, "'pingbacks'=>true") || strings.Contains(s, `"pingbacks"=>true`),
		UploadsPHP: strings.Contains(s, "'uploadsphp'=>true") || strings.Contains(s, `"uploadsphp"=>true`),
	}
}

func writeWPHarden(user, root string, h wpHarden) error {
	dir := wpHardenDir(root)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	plugin := filepath.Join(dir, "siroc-harden.php")
	if err := os.WriteFile(plugin, []byte(wpHardenPlugin), 0644); err != nil {
		return err
	}
	body := fmt.Sprintf("<?php\nreturn array('xmlrpc'=>%s,'pingbacks'=>%s,'uploadsphp'=>%s);\n", phpBool(h.XMLRPC), phpBool(h.Pingbacks), phpBool(h.UploadsPHP))
	if err := os.WriteFile(wpHardenSettingsPath(root), []byte(body), 0644); err != nil {
		return err
	}
	own := user + ":" + user
	_ = exec.Command("chown", "-R", own, dir).Run()
	return nil
}

func xmlrpcSnippetPath(domain string) string {
	return filepath.Join("/etc/nginx/snippets", "siroc-xmlrpc-"+domain+".conf")
}

func ensureXMLRPCSnippet(domain string) error {
	if err := validate.Domain(domain); err != nil {
		return err
	}
	if err := os.MkdirAll("/etc/nginx/snippets", 0755); err != nil {
		return err
	}
	path := xmlrpcSnippetPath(domain)
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte("# Siroc xmlrpc control\n"), 0644)
}

func writeXMLRPCSnippet(domain string, block bool) error {
	if err := os.MkdirAll("/etc/nginx/snippets", 0755); err != nil {
		return err
	}
	body := "# Siroc xmlrpc control\n"
	if block {
		body = "location = /xmlrpc.php { return 403; }\n"
	}
	return os.WriteFile(xmlrpcSnippetPath(domain), []byte(body), 0644)
}

func nginxXMLRPCBlocked(domain string) bool {
	b, err := os.ReadFile(xmlrpcSnippetPath(domain))
	if err != nil {
		return false
	}
	return strings.Contains(string(b), "xmlrpc.php")
}

func ensureNginxXMLRPCInclude(domain string) error {
	path := filepath.Join("/etc/nginx/sites-available", domain+".conf")
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	needle := "siroc-xmlrpc-" + domain + ".conf"
	if strings.Contains(string(b), needle) {
		return nil
	}
	line := "    include /etc/nginx/snippets/" + needle + ";\n"
	s := strings.ReplaceAll(string(b), "    location ^~ /fpm-status { return 404; }\n", "    location ^~ /fpm-status { return 404; }\n"+line)
	if s == string(b) {
		s = strings.ReplaceAll(string(b), "    location / {", line+"    location / {")
	}
	if s == string(b) {
		return fmt.Errorf("could not insert xmlrpc include into nginx config")
	}
	return os.WriteFile(path, []byte(s), 0644)
}

func setWPXMLRPC(user, root, domain string, block bool) error {
	h := readWPHarden(root)
	h.XMLRPC = block
	if err := writeWPHarden(user, root, h); err != nil {
		return err
	}
	if err := writeXMLRPCSnippet(domain, block); err != nil {
		return err
	}
	if err := ensureNginxXMLRPCInclude(domain); err != nil {
		return err
	}
	return testReload("nginx", "nginx", "-t")
}

func setWPPingbacks(user, root string, block bool) error {
	h := readWPHarden(root)
	h.Pingbacks = block
	if err := writeWPHarden(user, root, h); err != nil {
		return err
	}
	status := "open"
	flag := "1"
	if block {
		status = "closed"
		flag = "0"
	}
	if _, err := wpRun(user, root, "option", "update", "default_ping_status", status); err != nil {
		return err
	}
	if _, err := wpRun(user, root, "option", "update", "default_pingback_flag", flag); err != nil {
		return err
	}
	if !block {
		return nil
	}
	raw, err := wpRun(user, root, "post", "list", "--post_type=post,page", "--format=ids")
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var ids []string
	for _, id := range strings.Fields(raw) {
		if wpPostIDRe.MatchString(id) {
			ids = append(ids, id)
		}
		if len(ids) >= 200 {
			break
		}
	}
	if len(ids) == 0 {
		return nil
	}
	args := append([]string{"post", "update"}, ids...)
	args = append(args, "--ping_status=closed")
	_, _ = wpRun(user, root, args...)
	return nil
}

func fillWPHarden(st *rpc.WPStatus, root, domain string) {
	if st == nil {
		return
	}
	h := readWPHarden(root)
	st.XMLRPCBlocked = h.XMLRPC || nginxXMLRPCBlocked(domain)
	st.PingbacksOff = h.Pingbacks
	st.UploadsPHPBlocked = h.UploadsPHP || nginxUploadsPHPBlocked(domain)
}

func renameWPUser(user, root string, id int64, login string) (string, error) {
	if id < 1 {
		return "", fmt.Errorf("user id required")
	}
	login, err := validWPLogin(login)
	if err != nil {
		return "", err
	}
	st, err := wpStatus(user, root, "")
	if err != nil {
		return "", err
	}
	var found bool
	for _, u := range st.Users {
		if u.ID == id {
			found = true
			if u.Login == login {
				return "login is already " + login, nil
			}
		}
		if u.ID != id && strings.EqualFold(u.Login, login) {
			return "", fmt.Errorf("username %s is already taken", login)
		}
	}
	if !found {
		return "", fmt.Errorf("user %d not found", id)
	}
	prefix := strings.TrimSpace(st.Prefix)
	if prefix == "" {
		prefix = strings.TrimSpace(mustWP(user, root, "config", "get", "table_prefix", "--type=variable"))
	}
	if !wpPrefixRe.MatchString(prefix) {
		return "", fmt.Errorf("could not read table prefix")
	}
	tbl, err := sqlIdent(prefix + "users")
	if err != nil {
		return "", err
	}
	sql := fmt.Sprintf(
		"UPDATE %s SET user_login=%s, user_nicename=%s WHERE ID=%d LIMIT 1",
		tbl, sqlString(login), sqlString(wpNicename(login)), id,
	)
	if msg, err := mysqlExec(user, root, sql); err != nil {
		return msg, err
	}
	_, _ = wpRun(user, root, "cache", "flush")
	return "username is now " + login, nil
}

func dropXMLRPCSnippet(domain string) {
	_ = os.Remove(xmlrpcSnippetPath(domain))
	_ = os.Remove(uploadsPHPSnippetPath(domain))
}

func uploadsPHPSnippetPath(domain string) string {
	return filepath.Join("/etc/nginx/snippets", "siroc-uploads-php-"+domain+".conf")
}

func ensureUploadsPHPSnippet(domain string) error {
	if err := validate.Domain(domain); err != nil {
		return err
	}
	if err := os.MkdirAll("/etc/nginx/snippets", 0755); err != nil {
		return err
	}
	path := uploadsPHPSnippetPath(domain)
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte("# Siroc uploads PHP control\n"), 0644)
}

func writeUploadsPHPSnippet(domain string, block bool) error {
	if err := os.MkdirAll("/etc/nginx/snippets", 0755); err != nil {
		return err
	}
	body := "# Siroc uploads PHP control\n"
	if block {
		body = `location ~* ^/wp-content/uploads/.*\.(?:php[3457]?|phtml|phar|pht)(/|$) { return 403; }
location ~* ^/uploads/.*\.(?:php[3457]?|phtml|phar|pht)(/|$) { return 403; }
`
	}
	return os.WriteFile(uploadsPHPSnippetPath(domain), []byte(body), 0644)
}

func nginxUploadsPHPBlocked(domain string) bool {
	b, err := os.ReadFile(uploadsPHPSnippetPath(domain))
	if err != nil {
		return false
	}
	return strings.Contains(string(b), "wp-content/uploads")
}

func ensureNginxUploadsPHPInclude(domain string) error {
	path := filepath.Join("/etc/nginx/sites-available", domain+".conf")
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	needle := "siroc-uploads-php-" + domain + ".conf"
	s := string(b)
	if strings.Contains(s, needle) {
		return nil
	}
	line := "    include /etc/nginx/snippets/" + needle + ";\n"
	xmlrpc := "    include /etc/nginx/snippets/siroc-xmlrpc-" + domain + ".conf;\n"
	if strings.Contains(s, xmlrpc) {
		s = strings.ReplaceAll(s, xmlrpc, xmlrpc+line)
	} else {
		s = strings.ReplaceAll(s, "    location ^~ /fpm-status { return 404; }\n", "    location ^~ /fpm-status { return 404; }\n"+line)
		if s == string(b) {
			s = strings.ReplaceAll(s, "    location / {", line+"    location / {")
		}
	}
	if s == string(b) {
		return fmt.Errorf("could not insert uploads PHP include into nginx config")
	}
	return os.WriteFile(path, []byte(s), 0644)
}

const uploadsPHPHtaccess = `# BEGIN Siroc uploads-php
<FilesMatch "\.(?:php[0-9]?|phtml|phar|pht)$">
    Require all denied
</FilesMatch>
# END Siroc uploads-php
`

func writeUploadsPHPHtaccess(user, root string, block bool) error {
	dir := filepath.Join(root, "wp-content", "uploads")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, ".htaccess")
	cur := ""
	if b, err := os.ReadFile(path); err == nil {
		cur = string(b)
	}
	cur = stripUploadsPHPHtaccess(cur)
	if block {
		if strings.TrimSpace(cur) != "" && !strings.HasSuffix(cur, "\n") {
			cur += "\n"
		}
		cur += uploadsPHPHtaccess
		if !strings.HasSuffix(cur, "\n") {
			cur += "\n"
		}
	}
	if strings.TrimSpace(cur) == "" {
		_ = os.Remove(path)
		return nil
	}
	if err := os.WriteFile(path, []byte(cur), 0644); err != nil {
		return err
	}
	_ = exec.Command("chown", user+":"+user, path).Run()
	_ = exec.Command("chown", user+":"+user, dir).Run()
	return nil
}

func stripUploadsPHPHtaccess(s string) string {
	for _, pair := range [][2]string{
		{"# BEGIN Siroc uploads-php", "# END Siroc uploads-php"},
		{"# BEGIN CP Server uploads-php", "# END CP Server uploads-php"},
	} {
		start := strings.Index(s, pair[0])
		end := strings.Index(s, pair[1])
		if start < 0 || end < 0 || end < start {
			continue
		}
		end += len(pair[1])
		if end < len(s) && s[end] == '\n' {
			end++
		}
		s = strings.TrimSpace(s[:start] + s[end:])
	}
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return strings.TrimSpace(s) + "\n"
}

func setWPUploadsPHP(user, root, domain string, block bool) error {
	h := readWPHarden(root)
	h.UploadsPHP = block
	if err := writeWPHarden(user, root, h); err != nil {
		return err
	}
	if err := writeUploadsPHPSnippet(domain, block); err != nil {
		return err
	}
	if err := ensureNginxUploadsPHPInclude(domain); err != nil {
		return err
	}
	if err := writeUploadsPHPHtaccess(user, root, block); err != nil {
		return err
	}
	return testReload("nginx", "nginx", "-t")
}

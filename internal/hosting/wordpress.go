//go:build linux

package hosting

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

var wpPluginRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

const wpAutologinPlugin = `<?php
/**
 * Plugin Name: Siroc Autologin
 */
add_action('plugins_loaded', function () {
    if (empty($_GET['cp_login']) || is_user_logged_in()) {
        return;
    }
    $token = preg_replace('/[^a-f0-9]/', '', strtolower((string) $_GET['cp_login']));
    if (strlen($token) !== 32) {
        return;
    }
    $file = __DIR__ . '/siroc-autologin-token.php';
    if (!is_readable($file)) {
        return;
    }
    $data = include $file;
    @unlink($file);
    if (!is_array($data) || empty($data['t']) || empty($data['u']) || empty($data['e'])) {
        return;
    }
    if (!hash_equals((string) $data['t'], $token) || time() > (int) $data['e']) {
        return;
    }
    $user = get_user_by('id', (int) $data['u']);
    if (!$user) {
        return;
    }
    wp_set_current_user($user->ID);
    wp_set_auth_cookie($user->ID, true);
    wp_safe_redirect(admin_url());
    exit;
});
`

func ensureWPCLI() error {
	if _, err := exec.LookPath("wp"); err == nil {
		return nil
	}
	if err := exec.Command("curl", "-fsSL", "-o", "/usr/local/bin/wp", "https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar").Run(); err != nil {
		return fmt.Errorf("download wp-cli: %w", err)
	}
	return os.Chmod("/usr/local/bin/wp", 0755)
}

func wpRun(user, root string, args ...string) (string, error) {
	if err := ensureWPCLI(); err != nil {
		return "", err
	}
	return runAs(user, root, 8*time.Minute, "wp --path="+shellQuote(root)+" "+strings.Join(args, " "))
}

func wpPlugin(name string) (string, error) {
	n := strings.TrimSpace(name)
	if !wpPluginRe.MatchString(n) {
		return "", fmt.Errorf("invalid plugin slug")
	}
	return n, nil
}

func wpJSON(user, root string, dest any, args ...string) error {
	out, err := wpRun(user, root, args...)
	if err != nil {
		return err
	}
	out = strings.TrimSpace(out)
	if dest == nil || out == "" || out == "null" {
		return nil
	}
	if i := strings.IndexAny(out, "[{"); i > 0 {
		out = out[i:]
	}
	if err := json.Unmarshal([]byte(out), dest); err != nil {
		return fmt.Errorf("wp-cli json: %s", tailOut(out))
	}
	return nil
}

func wpStatus(user, root, domain string) (*rpc.WPStatus, error) {
	st := &rpc.WPStatus{}
	st.Name, _ = wpRun(user, root, "option", "get", "blogname")
	st.URL, _ = wpRun(user, root, "option", "get", "siteurl")
	st.Home, _ = wpRun(user, root, "option", "get", "home")
	st.Version, _ = wpRun(user, root, "core", "version")
	st.Name = strings.TrimSpace(st.Name)
	st.URL = strings.TrimSpace(st.URL)
	st.Home = strings.TrimSpace(st.Home)
	st.Version = strings.TrimSpace(st.Version)
	st.Prefix = strings.TrimSpace(mustWP(user, root, "db", "prefix"))
	var users []map[string]any
	if err := wpJSON(user, root, &users, "user", "list", "--fields=ID,user_login,user_email,display_name,roles", "--format=json"); err == nil {
		for _, u := range users {
			st.Users = append(st.Users, rpc.WPUser{
				ID:    toInt64(u["ID"]),
				Login: fmt.Sprint(u["user_login"]),
				Email: fmt.Sprint(u["user_email"]),
				Name:  fmt.Sprint(u["display_name"]),
				Roles: fmt.Sprint(u["roles"]),
			})
		}
	}
	var plugs []map[string]any
	if err := wpJSON(user, root, &plugs, "plugin", "list", "--fields=name,status,version,update,title", "--format=json"); err == nil {
		for _, p := range plugs {
			st.Plugins = append(st.Plugins, rpc.WPPlugin{
				Name:    fmt.Sprint(p["name"]),
				Status:  fmt.Sprint(p["status"]),
				Version: fmt.Sprint(p["version"]),
				Update:  fmt.Sprint(p["update"]),
				Title:   fmt.Sprint(p["title"]),
			})
		}
	}
	fillWPHarden(st, root, domain)
	return st, nil
}

func wpInstall(req rpc.SiteAppReq, out *rpc.SiteAppResp) (*rpc.SiteAppResp, error) {
	root := out.DocRoot
	if root == "" {
		root = out.AppRoot
	}
	if fileOK(filepath.Join(root, "wp-config.php")) {
		return nil, fmt.Errorf("WordPress is already installed in this document root")
	}
	if err := ensureWPCLI(); err != nil {
		return nil, err
	}
	title := strings.TrimSpace(req.Name)
	if title == "" {
		title = req.Domain
	}
	siteURL := strings.TrimSpace(req.URL)
	if siteURL == "" {
		siteURL = "http://" + req.Domain
	}
	admin, err := validWPLogin(req.AdminUser)
	if err != nil {
		return nil, err
	}
	pass := strings.TrimSpace(req.AdminPass)
	if len(pass) < 8 {
		return nil, fmt.Errorf("admin password must be at least 8 characters")
	}
	email := strings.TrimSpace(req.AdminEmail)
	if email == "" {
		email = admin + "@" + req.Domain
	}
	dbName := strings.TrimSpace(req.DBName)
	dbUser := strings.TrimSpace(req.DBUser)
	dbPass := strings.TrimSpace(req.DBPass)
	if dbName == "" || dbUser == "" || dbPass == "" {
		return nil, fmt.Errorf("database name, user, and password are required")
	}
	prefix, err := validWPPrefix(req.Prefix)
	if err != nil {
		return nil, err
	}
	if _, err := wpRun(req.Username, root, "core", "download", "--force"); err != nil {
		return nil, fmt.Errorf("wp download: %w", err)
	}
	cfg := []string{
		"config", "create",
		"--dbname=" + shellQuote(dbName),
		"--dbuser=" + shellQuote(dbUser),
		"--dbpass=" + shellQuote(dbPass),
		"--dbhost=localhost",
		"--dbprefix=" + shellQuote(prefix),
		"--skip-check",
	}
	if _, err := wpRun(req.Username, root, cfg...); err != nil {
		return nil, fmt.Errorf("wp config: %w", err)
	}
	inst := []string{
		"core", "install",
		"--url=" + shellQuote(siteURL),
		"--title=" + shellQuote(title),
		"--admin_user=" + shellQuote(admin),
		"--admin_password=" + shellQuote(pass),
		"--admin_email=" + shellQuote(email),
		"--skip-email",
	}
	msg, err := wpRun(req.Username, root, inst...)
	if err != nil {
		return nil, fmt.Errorf("wp install: %w", err)
	}
	out.Kind = "wordpress"
	out.AppRoot = root
	out.Message = msg
	wp, _ := wpStatus(req.Username, root, req.Domain)
	if wp != nil {
		wp.WPCmds = listWPCLI(req.Username, root)
	}
	out.WordPress = wp
	return out, nil
}

func wpAction(req rpc.SiteAppReq, out *rpc.SiteAppResp) (*rpc.SiteAppResp, error) {
	root := out.AppRoot
	user := req.Username
	switch req.Action {
	case "settings":
		if strings.TrimSpace(req.Name) != "" {
			if _, err := wpRun(user, root, "option", "update", "blogname", shellQuote(req.Name)); err != nil {
				return nil, err
			}
		}
		if u := strings.TrimSpace(req.URL); u != "" {
			if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
				return nil, fmt.Errorf("url must start with http:// or https://")
			}
			if _, err := wpRun(user, root, "option", "update", "siteurl", shellQuote(u)); err != nil {
				return nil, err
			}
			if _, err := wpRun(user, root, "option", "update", "home", shellQuote(u)); err != nil {
				return nil, err
			}
		}
	case "plugin-on":
		plug, err := wpPlugin(req.Plugin)
		if err != nil {
			return nil, err
		}
		if _, err := wpRun(user, root, "plugin", "activate", plug); err != nil {
			return nil, err
		}
	case "plugin-off":
		plug, err := wpPlugin(req.Plugin)
		if err != nil {
			return nil, err
		}
		if _, err := wpRun(user, root, "plugin", "deactivate", plug); err != nil {
			return nil, err
		}
	case "plugin-uninstall":
		plug, err := wpPlugin(req.Plugin)
		if err != nil {
			return nil, err
		}
		_, _ = wpRun(user, root, "plugin", "deactivate", plug)
		if msg, err := wpRun(user, root, "plugin", "uninstall", "--deactivate", plug); err != nil {
			out.Output = msg
			return nil, err
		}
	case "updates":
		st, err := wpStatus(user, root, req.Domain)
		if err != nil {
			return nil, err
		}
		st.Updates = &rpc.WPUpdates{}
		core, _ := wpRun(user, root, "core", "check-update", "--format=json")
		st.Updates.Core = strings.TrimSpace(core)
		var plugs []map[string]any
		if err := wpJSON(user, root, &plugs, "plugin", "list", "--update=available", "--fields=name,title,version,update_version", "--format=json"); err == nil {
			for _, p := range plugs {
				st.Updates.Plugins = append(st.Updates.Plugins, rpc.WPPlugin{
					Name:    fmt.Sprint(p["name"]),
					Title:   fmt.Sprint(p["title"]),
					Version: fmt.Sprint(p["version"]),
					Update:  fmt.Sprint(p["update_version"]),
				})
			}
		}
		themes, _ := wpRun(user, root, "theme", "list", "--update=available", "--field=name")
		if strings.TrimSpace(themes) != "" {
			st.Updates.Themes = strings.Split(strings.TrimSpace(themes), "\n")
		}
		out.WordPress = st
		return out, nil
	case "update-core":
		msg, err := wpRun(user, root, "core", "update")
		out.Output = msg
		if err != nil {
			return nil, err
		}
	case "update-plugins":
		msg, err := wpRun(user, root, "plugin", "update", "--all")
		out.Output = msg
		if err != nil {
			return nil, err
		}
	case "login":
		urls, err := wpLoginAs(user, root, req.Domain, req.UserID)
		if err != nil {
			return nil, err
		}
		out.LoginURLs = urls
	case "security":
		st, err := wpStatus(user, root, req.Domain)
		if err != nil {
			return nil, err
		}
		st.Security = wpSecurity(user, root, st)
		out.WordPress = st
		return out, nil
	case "wpcli":
		script, err := wpCLIScript(root, req.WPCLI)
		if err != nil {
			return nil, err
		}
		msg, err := runAs(user, root, 5*time.Minute, script)
		out.Output = msg
		if err != nil {
			if out.Output == "" {
				out.Output = err.Error()
			}
			out.Message = "wp-cli exited with an error"
		}
		st, _ := wpStatus(user, root, req.Domain)
		out.WordPress = st
		return out, nil
	case "prefix":
		msg, err := changeWPPrefix(user, root, req.Prefix)
		out.Output = msg
		if err != nil {
			if out.Output == "" {
				out.Output = err.Error()
			}
			out.Message = err.Error()
			st, _ := wpStatus(user, root, req.Domain)
			out.WordPress = st
			return out, nil
		}
	case "xmlrpc":
		on := req.XMLRPC != nil && *req.XMLRPC
		if err := setWPXMLRPC(user, root, req.Domain, on); err != nil {
			return nil, err
		}
		if on {
			out.Message = "xmlrpc.php is blocked"
		} else {
			out.Message = "xmlrpc.php is allowed"
		}
	case "pingbacks":
		on := req.Pingbacks != nil && *req.Pingbacks
		if err := setWPPingbacks(user, root, on); err != nil {
			return nil, err
		}
		if on {
			out.Message = "pingbacks are blocked"
		} else {
			out.Message = "pingbacks are allowed"
		}
	case "uploads-php":
		on := req.UploadsPHP != nil && *req.UploadsPHP
		if err := setWPUploadsPHP(user, root, req.Domain, on); err != nil {
			return nil, err
		}
		if on {
			out.Message = "PHP is blocked in the uploads folder"
		} else {
			out.Message = "PHP is allowed in the uploads folder"
		}
	case "rename-user":
		msg, err := renameWPUser(user, root, req.UserID, req.Login)
		out.Output = msg
		if err != nil {
			if out.Output == "" {
				out.Output = err.Error()
			}
			out.Message = err.Error()
			st, _ := wpStatus(user, root, req.Domain)
			out.WordPress = st
			return out, nil
		}
	default:
		return nil, fmt.Errorf("unknown wordpress action %q", req.Action)
	}
	st, err := wpStatus(user, root, req.Domain)
	if err != nil {
		out.Message = err.Error()
	}
	out.WordPress = st
	return out, nil
}

func wpLoginAs(user, root, domain string, id int64) ([]string, error) {
	if id < 1 {
		return nil, fmt.Errorf("user id required")
	}
	mu := filepath.Join(root, "wp-content", "mu-plugins")
	if err := os.MkdirAll(mu, 0755); err != nil {
		return nil, err
	}
	plugin := filepath.Join(mu, "siroc-autologin.php")
	if err := os.WriteFile(plugin, []byte(wpAutologinPlugin), 0644); err != nil {
		return nil, err
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(b)
	body := fmt.Sprintf("<?php\nreturn array('t'=>'%s','u'=>%d,'e'=>%d);\n", token, id, time.Now().Add(2*time.Minute).Unix())
	tok := filepath.Join(mu, "siroc-autologin-token.php")
	if err := os.WriteFile(tok, []byte(body), 0644); err != nil {
		return nil, err
	}
	own := user + ":" + user
	_ = exec.Command("chown", "-R", own, mu).Run()
	return []string{
		"http://" + domain + "/?cp_login=" + token,
		"http://" + domain + ":8080/?cp_login=" + token,
		"https://" + domain + "/?cp_login=" + token,
		"https://" + domain + ":8444/?cp_login=" + token,
	}, nil
}

func wpSecurity(user, root string, st *rpc.WPStatus) []rpc.WPIssue {
	var issues []rpc.WPIssue
	debug, _ := wpRun(user, root, "config", "get", "WP_DEBUG")
	if strings.EqualFold(strings.TrimSpace(debug), "1") || strings.EqualFold(strings.TrimSpace(debug), "true") {
		issues = append(issues, rpc.WPIssue{ID: "debug", Level: "warn", Title: "WP_DEBUG is on", Detail: "Disable debugging on production sites."})
	}
	edit, _ := wpRun(user, root, "config", "get", "DISALLOW_FILE_EDIT")
	if strings.TrimSpace(edit) == "" || strings.EqualFold(strings.TrimSpace(edit), "false") || strings.TrimSpace(edit) == "0" {
		issues = append(issues, rpc.WPIssue{ID: "file-edit", Level: "warn", Title: "Theme/plugin file editor is enabled", Detail: "Set DISALLOW_FILE_EDIT to true in wp-config.php."})
	}
	for _, u := range st.Users {
		if strings.EqualFold(u.Login, "admin") {
			issues = append(issues, rpc.WPIssue{ID: "admin-user", Level: "warn", Title: "User named admin exists", Detail: "Use Randomize login on the Users tab."})
		}
	}
	for _, p := range st.Plugins {
		if p.Update == "available" {
			issues = append(issues, rpc.WPIssue{ID: "plugin-" + p.Name, Level: "warn", Title: "Plugin update available: " + p.Title, Detail: p.Name + " " + p.Version})
		}
	}
	if st.URL != "" && strings.HasPrefix(strings.ToLower(st.URL), "http://") {
		issues = append(issues, rpc.WPIssue{ID: "ssl", Level: "info", Title: "Site URL is HTTP", Detail: st.URL})
	}
	cfg := filepath.Join(root, "wp-config.php")
	if fi, err := os.Stat(cfg); err == nil && fi.Mode().Perm()&0007 != 0 {
		issues = append(issues, rpc.WPIssue{ID: "config-perm", Level: "warn", Title: "wp-config.php is world-readable", Detail: fi.Mode().String()})
	}
	prefix, _ := wpRun(user, root, "db", "prefix")
	if strings.TrimSpace(prefix) == "wp_" {
		issues = append(issues, rpc.WPIssue{ID: "prefix", Level: "info", Title: "Default table prefix wp_", Detail: "A custom prefix makes some automated attacks harder."})
	}
	if fileOK(filepath.Join(root, "xmlrpc.php")) && !st.XMLRPCBlocked {
		issues = append(issues, rpc.WPIssue{ID: "xmlrpc", Level: "warn", Title: "xmlrpc.php is open", Detail: "Block it from the Security tab unless you need the WordPress app."})
	}
	if !st.PingbacksOff {
		issues = append(issues, rpc.WPIssue{ID: "pingbacks", Level: "info", Title: "Pingbacks are enabled", Detail: "Incoming pingbacks use xmlrpc.php and can be used for DDoS amplification."})
	}
	if !st.UploadsPHPBlocked {
		issues = append(issues, rpc.WPIssue{ID: "uploads-php", Level: "warn", Title: "PHP can run in uploads", Detail: "Block PHP in wp-content/uploads from the Security tab."})
	}
	if len(issues) == 0 {
		issues = append(issues, rpc.WPIssue{ID: "ok", Level: "ok", Title: "No obvious issues", Detail: "Core checks passed."})
	}
	_ = user
	return issues
}

func toInt64(v any) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case int64:
		return t
	case int:
		return int64(t)
	case string:
		n, _ := strconv.ParseInt(t, 10, 64)
		return n
	case json.Number:
		n, _ := t.Int64()
		return n
	default:
		n, _ := strconv.ParseInt(fmt.Sprint(v), 10, 64)
		return n
	}
}

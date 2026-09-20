//go:build linux

package hosting

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/security"
	"github.com/siroc-dev/siroc/internal/validate"
	"github.com/siroc-dev/siroc/internal/weblog"
)

type Manager struct {
	HomeRoot    string
	NginxSites  string
	ApacheSites string
}

type siteData struct {
	Username          string
	Domain            string
	DocRoot           string
	PHPVersion        string
	PHPSocket         string
	Enabled           bool
	PM                string
	MaxChildren       int
	StartServers      int
	MinSpare          int
	MaxSpare          int
	IdleTimeout       string
	MaxRequests       int
	MemoryLimit       string
	MaxExecutionTime  int
	MaxInputTime      int
	PostMaxSize       string
	UploadMaxFilesize string
	DisplayErrors     string
	Timezone          string
	DisableFunctions  string
	OpenBasedir       string
	UserTmp           string
	PHPIniDir         string
	AllNames          string
	AliasLine         string
	SSL               bool
	SSLRedirect       bool
	SSLCert           string
	SSLKey            string
	Rewrite           string
	ProxyPass         string
	WAF               bool
}

const nginxTmpl = `server {
    listen 80;
    listen [::]:80;
    server_name {{.AllNames}};
    root {{.DocRoot}};
    client_max_body_size {{.PostMaxSize}};
    access_log /var/log/nginx/sites/{{.Domain}}-access.log combined;
    error_log /var/log/nginx/sites/{{.Domain}}-error.log;

    location ^~ /.well-known/acme-challenge/ {
        root /var/www/letsencrypt;
        default_type text/plain;
    }
    location ^~ /server-status { return 404; }
    location ^~ /nginx-status { return 404; }
    location ^~ /fpm-status { return 404; }
    include /etc/nginx/snippets/siroc-xmlrpc-{{.Domain}}.conf;
    include /etc/nginx/snippets/siroc-uploads-php-{{.Domain}}.conf;
{{- if .SSLRedirect}}
    location / {
        return 301 https://$host$request_uri;
    }
{{- else}}
    location / {
{{- if .ProxyPass}}
        proxy_pass {{.ProxyPass}};
        proxy_http_version 1.1;
        proxy_set_header Host $http_host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 300s;
{{- else}}
{{.Rewrite}}        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $http_host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
{{- end}}
    }
{{- if not .ProxyPass}}
    location ~ \.php$ {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $http_host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
{{- end}}
{{- end}}
}
{{- if .SSL}}

server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name {{.AllNames}};
    root {{.DocRoot}};
    client_max_body_size {{.PostMaxSize}};
    ssl_certificate {{.SSLCert}};
    ssl_certificate_key {{.SSLKey}};
    ssl_session_timeout 1d;
    ssl_session_cache shared:SSL:10m;
    ssl_protocols TLSv1.2 TLSv1.3;
    access_log /var/log/nginx/sites/{{.Domain}}-access.log combined;
    error_log /var/log/nginx/sites/{{.Domain}}-error.log;

    location ^~ /server-status { return 404; }
    location ^~ /nginx-status { return 404; }
    location ^~ /fpm-status { return 404; }
    include /etc/nginx/snippets/siroc-xmlrpc-{{.Domain}}.conf;
    include /etc/nginx/snippets/siroc-uploads-php-{{.Domain}}.conf;

    location / {
{{- if .ProxyPass}}
        proxy_pass {{.ProxyPass}};
        proxy_http_version 1.1;
        proxy_set_header Host $http_host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 300s;
{{- else}}
{{.Rewrite}}        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $http_host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
{{- end}}
    }
{{- if not .ProxyPass}}
    location ~ \.php$ {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $http_host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
    }
{{- end}}
}
{{- end}}
`

const apacheTmpl = `<VirtualHost 127.0.0.1:8080>
    ServerName {{.Domain}}
{{- if .AliasLine}}
    ServerAlias {{.AliasLine}}
{{- end}}
    UseCanonicalName Off
    UseCanonicalPhysicalPort Off
    SetEnvIf X-Forwarded-Proto "https" HTTPS=on
    DocumentRoot {{.DocRoot}}
    <Directory {{.DocRoot}}>
        Options FollowSymLinks
        AllowOverride All
        Require all granted
    </Directory>
{{- if .PHPSocket}}
    <FilesMatch \.php$>
        SetHandler "proxy:unix:{{.PHPSocket}}|fcgi://localhost"
    </FilesMatch>
{{- end}}
    ErrorLog ${APACHE_LOG_DIR}/sites/{{.Domain}}-error.log
    CustomLog ${APACHE_LOG_DIR}/sites/{{.Domain}}-access.log combined
    <IfModule security2_module>
        SecAuditLogType Serial
        SecAuditLog ${APACHE_LOG_DIR}/sites/{{.Domain}}-modsec.log
{{- if not .WAF}}
        SecRuleEngine Off
{{- end}}
    </IfModule>
</VirtualHost>
`

const phpPoolTmpl = `[{{.Username}}-{{.Domain}}]
user = {{.Username}}
group = {{.Username}}
listen = {{.PHPSocket}}
listen.owner = www-data
listen.group = www-data
listen.mode = 0660
pm = {{.PM}}
pm.max_children = {{.MaxChildren}}
{{- if eq .PM "dynamic"}}
pm.start_servers = {{.StartServers}}
pm.min_spare_servers = {{.MinSpare}}
pm.max_spare_servers = {{.MaxSpare}}
{{- else if eq .PM "ondemand"}}
pm.process_idle_timeout = {{.IdleTimeout}}
{{- end}}
pm.max_requests = {{.MaxRequests}}
pm.status_path = /fpm-status
chdir = /
php_admin_value[memory_limit] = {{.MemoryLimit}}
php_admin_value[max_execution_time] = {{.MaxExecutionTime}}
php_admin_value[max_input_time] = {{.MaxInputTime}}
php_admin_value[post_max_size] = {{.PostMaxSize}}
php_admin_value[upload_max_filesize] = {{.UploadMaxFilesize}}
php_admin_flag[display_errors] = {{.DisplayErrors}}
php_admin_value[date.timezone] = {{.Timezone}}
{{- if .DisableFunctions}}
php_admin_value[disable_functions] = {{.DisableFunctions}}
{{- end}}
php_admin_value[open_basedir] = {{.OpenBasedir}}
php_admin_value[session.save_path] = {{.UserTmp}}
php_admin_value[upload_tmp_dir] = {{.UserTmp}}
php_admin_value[sys_temp_dir] = {{.UserTmp}}
{{- if .PHPIniDir}}
env[PHPRC] = {{.PHPIniDir}}
env[PHP_INI_SCAN_DIR] = :{{.PHPIniDir}}/conf.d
{{- end}}
`

func (m *Manager) Write(req rpc.SiteWriteReq) error {
	if err := validate.LinuxUser(req.Username); err != nil {
		return err
	}
	if err := validate.Domain(req.Domain); err != nil {
		return err
	}
	if validate.IsAppKind(req.Kind) {
		return m.writeApp(req)
	}
	dropApp(req.Username, req.Domain)
	if strings.EqualFold(strings.TrimSpace(req.Kind), "proxy") || strings.TrimSpace(req.ProxyPass) != "" {
		return m.writeProxy(req)
	}
	if err := validate.PHPVersion(req.PHPVersion); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join("/etc/php", req.PHPVersion, "fpm")); err != nil {
		return fmt.Errorf("PHP %s-FPM is not installed", req.PHPVersion)
	}
	doc, err := validate.AccountPath(m.HomeRoot, req.Username, req.DocRoot, req.Domain)
	if err != nil {
		return err
	}
	req.DocRoot = doc
	if err := os.MkdirAll(doc, 0755); err != nil {
		return err
	}
	idx := filepath.Join(doc, "index.php")
	if _, err := os.Stat(idx); os.IsNotExist(err) {
		_ = os.WriteFile(idx, []byte("<?php phpinfo(); ?>"), 0644)
	}
	_, tmp, err := validate.HomeJail(m.HomeRoot, req.Username)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(tmp, 0750); err != nil {
		return err
	}
	own := req.Username + ":" + req.Username
	_ = exec.Command("chown", "-R", own, doc).Run()
	_ = exec.Command("chown", own, tmp).Run()
	_ = m.FixWebPerms(req.Username, doc)

	sock := fmt.Sprintf("/run/php/php%s-%s-%s.sock", req.PHPVersion, req.Username, slug(req.Domain))
	aliases, err := validate.DomainAliases(req.Domain, req.Aliases)
	if err != nil {
		return err
	}
	snippet, err := validate.NginxSnippet(req.Rewrite)
	if err != nil {
		return err
	}
	if err := os.MkdirAll("/var/www/letsencrypt/.well-known/acme-challenge", 0755); err != nil {
		return err
	}
	if err := ensureLocalCert(req.Domain, aliases); err != nil {
		return err
	}
	kind, cert, key := resolveCerts(req.Domain, req.SSLKind)
	ssl := req.SSL && cert != ""
	data := siteData{
		Username:    req.Username,
		Domain:      req.Domain,
		DocRoot:     doc,
		PHPVersion:  req.PHPVersion,
		PHPSocket:   sock,
		Enabled:     req.Enabled,
		AllNames:    strings.Join(append([]string{req.Domain}, aliases...), " "),
		AliasLine:   strings.Join(aliases, " "),
		SSL:         ssl,
		SSLRedirect: ssl && kind == "letsencrypt",
		SSLCert:     cert,
		SSLKey:      key,
		Rewrite:     snippet,
		ProxyPass:   "",
		WAF:         req.WAF,
	}
	if err := fillSiteFPM(&data, m.HomeRoot, req); err != nil {
		return err
	}
	if req.FPM != nil {
		settings, err := normalizeFPM(*req.FPM)
		if err != nil {
			return err
		}
		if err := m.writeUserPHP(req.Username, req.PHPVersion, settings); err != nil {
			return err
		}
	}

	if err := weblog.TouchSiteLogs(req.Domain); err != nil {
		return err
	}
	if err := ensureXMLRPCSnippet(req.Domain); err != nil {
		return err
	}
	if err := ensureUploadsPHPSnippet(req.Domain); err != nil {
		return err
	}

	if err := writeTemplate(filepath.Join("/etc/nginx/sites-available", req.Domain+".conf"), nginxTmpl, data, 0644); err != nil {
		return err
	}
	if err := writeTemplate(filepath.Join("/etc/apache2/sites-available", req.Domain+".conf"), apacheTmpl, data, 0644); err != nil {
		return err
	}
	poolDir := fmt.Sprintf("/etc/php/%s/fpm/pool.d", req.PHPVersion)
	if err := os.MkdirAll(poolDir, 0755); err != nil {
		return err
	}
	poolName := fmt.Sprintf("%s-%s.conf", req.Username, slug(req.Domain))
	if err := writeTemplate(filepath.Join(poolDir, poolName), phpPoolTmpl, data, 0644); err != nil {
		return err
	}
	dropSitePools(req.Username, req.Domain, req.PHPVersion)

	if req.Enabled {
		_ = os.MkdirAll("/etc/nginx/sites-enabled", 0755)
		_ = os.Remove(filepath.Join("/etc/nginx/sites-enabled", req.Domain+".conf"))
		if err := os.Symlink(filepath.Join("/etc/nginx/sites-available", req.Domain+".conf"), filepath.Join("/etc/nginx/sites-enabled", req.Domain+".conf")); err != nil && !os.IsExist(err) {
			return err
		}
		if err := exec.Command("a2ensite", req.Domain+".conf").Run(); err != nil {
			return fmt.Errorf("a2ensite: %w", err)
		}
	} else {
		_ = os.Remove(filepath.Join("/etc/nginx/sites-enabled", req.Domain+".conf"))
		_ = exec.Command("a2dissite", req.Domain+".conf").Run()
	}

	if err := exec.Command("systemctl", "reload", "php"+req.PHPVersion+"-fpm").Run(); err != nil {
		return fmt.Errorf("reload php%s-fpm: %w", req.PHPVersion, err)
	}
	if err := testReload("nginx", "nginx", "-t"); err != nil {
		return err
	}
	if err := testReload("apache2", "apache2ctl", "configtest"); err != nil {
		return err
	}
	security.SyncLocalVhostHosts(req.Domain)
	return nil
}

func (m *Manager) writeProxy(req rpc.SiteWriteReq) error {
	pass, err := validate.ProxyURL(req.ProxyPass)
	if err != nil {
		return err
	}
	if req.PHPVersion == "" {
		req.PHPVersion = "8.3"
	}
	doc := req.DocRoot
	if strings.TrimSpace(doc) == "" {
		doc = filepath.Join(m.HomeRoot, req.Username, "domains", req.Domain, "public_html")
	} else {
		doc, err = validate.AccountPath(m.HomeRoot, req.Username, req.DocRoot, req.Domain)
		if err != nil {
			return err
		}
	}
	if err := os.MkdirAll(doc, 0755); err != nil {
		return err
	}
	_ = exec.Command("chown", "-R", req.Username+":"+req.Username, doc).Run()
	aliases, err := validate.DomainAliases(req.Domain, req.Aliases)
	if err != nil {
		return err
	}
	if err := os.MkdirAll("/var/www/letsencrypt/.well-known/acme-challenge", 0755); err != nil {
		return err
	}
	if err := ensureLocalCert(req.Domain, aliases); err != nil {
		return err
	}
	kind, cert, key := resolveCerts(req.Domain, req.SSLKind)
	ssl := req.SSL && cert != ""
	data := siteData{
		Username:    req.Username,
		Domain:      req.Domain,
		DocRoot:     doc,
		PHPVersion:  req.PHPVersion,
		Enabled:     req.Enabled,
		AllNames:    strings.Join(append([]string{req.Domain}, aliases...), " "),
		AliasLine:   strings.Join(aliases, " "),
		SSL:         ssl,
		SSLRedirect: ssl && kind == "letsencrypt",
		SSLCert:     cert,
		SSLKey:      key,
		ProxyPass:   pass,
		PostMaxSize: "64M",
	}
	if err := weblog.TouchSiteLogs(req.Domain); err != nil {
		return err
	}
	if err := ensureXMLRPCSnippet(req.Domain); err != nil {
		return err
	}
	if err := ensureUploadsPHPSnippet(req.Domain); err != nil {
		return err
	}
	if err := writeTemplate(filepath.Join("/etc/nginx/sites-available", req.Domain+".conf"), nginxTmpl, data, 0644); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join("/etc/apache2/sites-available", req.Domain+".conf"))
	_ = exec.Command("a2dissite", req.Domain+".conf").Run()
	dropSitePools(req.Username, req.Domain, "")
	if req.Enabled {
		_ = os.MkdirAll("/etc/nginx/sites-enabled", 0755)
		_ = os.Remove(filepath.Join("/etc/nginx/sites-enabled", req.Domain+".conf"))
		if err := os.Symlink(filepath.Join("/etc/nginx/sites-available", req.Domain+".conf"), filepath.Join("/etc/nginx/sites-enabled", req.Domain+".conf")); err != nil && !os.IsExist(err) {
			return err
		}
	} else {
		_ = os.Remove(filepath.Join("/etc/nginx/sites-enabled", req.Domain+".conf"))
	}
	if err := testReload("nginx", "nginx", "-t"); err != nil {
		return err
	}
	security.SyncLocalVhostHosts(req.Domain)
	return nil
}

func (m *Manager) Delete(username, domain string) error {
	if err := validate.LinuxUser(username); err != nil {
		return err
	}
	if err := validate.Domain(domain); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join("/etc/nginx/sites-enabled", domain+".conf"))
	_ = os.Remove(filepath.Join("/etc/nginx/sites-available", domain+".conf"))
	dropXMLRPCSnippet(domain)
	_ = os.RemoveAll(localSSLDir(domain))
	cleanupLaravel(username, domain)
	dropApp(username, domain)
	_ = exec.Command("a2dissite", domain+".conf").Run()
	_ = os.Remove(filepath.Join("/etc/apache2/sites-available", domain+".conf"))
	dropSitePools(username, domain, "")
	_ = testReload("nginx", "nginx", "-t")
	_ = testReload("apache2", "apache2ctl", "configtest")
	security.SyncLocalVhostHosts()
	return nil
}

func (m *Manager) Rename(req rpc.SiteRenameReq) error {
	old := strings.ToLower(strings.TrimSpace(req.OldDomain))
	newDom := strings.ToLower(strings.TrimSpace(req.Domain))
	if err := validate.LinuxUser(req.Username); err != nil {
		return err
	}
	if err := validate.Domain(old); err != nil {
		return fmt.Errorf("old domain: %w", err)
	}
	if err := validate.Domain(newDom); err != nil {
		return err
	}
	req.Domain = newDom
	if old == newDom {
		return m.Write(req.SiteWriteReq)
	}
	home, _, err := validate.HomeJail(m.HomeRoot, req.Username)
	if err != nil {
		return err
	}
	oldDir := filepath.Join(home, "domains", old)
	newDir := filepath.Join(home, "domains", newDom)
	moved := false
	if st, err := os.Stat(oldDir); err == nil && st.IsDir() {
		if _, err := os.Stat(newDir); err == nil {
			return fmt.Errorf("domains/%s already exists", newDom)
		}
		if err := os.Rename(oldDir, newDir); err != nil {
			return fmt.Errorf("rename domain directory: %w", err)
		}
		moved = true
	}
	_ = os.RemoveAll(localSSLDir(old))
	if b, err := os.ReadFile(xmlrpcSnippetPath(old)); err == nil {
		_ = os.MkdirAll("/etc/nginx/snippets", 0755)
		_ = os.WriteFile(xmlrpcSnippetPath(newDom), b, 0644)
	}
	if b, err := os.ReadFile(uploadsPHPSnippetPath(old)); err == nil {
		_ = os.MkdirAll("/etc/nginx/snippets", 0755)
		_ = os.WriteFile(uploadsPHPSnippetPath(newDom), b, 0644)
	}
	renameLaravel(req.Username, old, newDom)
	renameApp(req.Username, old, newDom)
	dropOldVhosts(req.Username, old)
	if err := m.Write(req.SiteWriteReq); err != nil {
		if moved {
			_ = os.Rename(newDir, oldDir)
		}
		return err
	}
	return nil
}

func dropOldVhosts(username, domain string) {
	_ = os.Remove(filepath.Join("/etc/nginx/sites-enabled", domain+".conf"))
	_ = os.Remove(filepath.Join("/etc/nginx/sites-available", domain+".conf"))
	dropXMLRPCSnippet(domain)
	_ = exec.Command("a2dissite", domain+".conf").Run()
	_ = os.Remove(filepath.Join("/etc/apache2/sites-available", domain+".conf"))
	dropSitePools(username, domain, "")
}

func dropSitePools(username, domain, keep string) {
	name := username + "-" + slug(domain) + ".conf"
	for _, v := range []string{"8.1", "8.2", "8.3", "8.4"} {
		if v == keep {
			continue
		}
		path := filepath.Join("/etc/php", v, "fpm/pool.d", name)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		_ = os.Remove(path)
		_ = exec.Command("systemctl", "reload", "php"+v+"-fpm").Run()
	}
}

func writeTemplate(path, tmpl string, data any, mode os.FileMode) error {
	t, err := template.New("cfg").Parse(tmpl)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), mode)
}

func testReload(service, bin string, args ...string) error {
	out, err := exec.Command(bin, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s config test: %s", service, strings.TrimSpace(string(out)))
	}
	if err := exec.Command("systemctl", "reload", service).Run(); err != nil {
		return fmt.Errorf("reload %s: %w", service, err)
	}
	return nil
}

func slug(domain string) string {
	return strings.ReplaceAll(domain, ".", "_")
}

func (m *Manager) FixWebPerms(username, doc string) error {
	home, _, err := validate.HomeJail(m.HomeRoot, username)
	if err != nil {
		return err
	}
	_ = os.Chmod(home, 0711)
	if doc == "" {
		return nil
	}
	abs := filepath.Clean(doc)
	for p := abs; p != home && p != "/" && p != "."; p = filepath.Dir(p) {
		_ = os.Chmod(p, 0755)
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
	}
	_ = os.Chmod(home, 0711)
	_ = filepath.WalkDir(abs, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			_ = os.Chmod(path, 0755)
		} else {
			_ = os.Chmod(path, 0644)
		}
		return nil
	})
	return nil
}

func patchConfFile(path string, reps [][2][]byte) (bool, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return false, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	n := b
	for _, r := range reps {
		n = bytes.ReplaceAll(n, r[0], r[1])
	}
	if bytes.Equal(b, n) {
		return false, nil
	}
	return true, os.WriteFile(path, n, st.Mode().Perm())
}

func ensureApacheForwardHeaders(path string) (bool, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return false, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	s := string(b)
	if !strings.Contains(s, "<VirtualHost") || strings.Contains(s, "UseCanonicalPhysicalPort") {
		return false, nil
	}
	var out []string
	inserted := false
	for _, line := range strings.Split(s, "\n") {
		out = append(out, line)
		if !inserted && strings.Contains(line, "ServerName ") {
			out = append(out,
				"    UseCanonicalName Off",
				"    UseCanonicalPhysicalPort Off",
				`    SetEnvIf X-Forwarded-Proto "https" HTTPS=on`,
			)
			inserted = true
		}
	}
	if !inserted {
		return false, nil
	}
	n := strings.Join(out, "\n")
	return true, os.WriteFile(path, []byte(n), st.Mode().Perm())
}

func fixProxyHostHeader() {
	changed := false
	for _, g := range []string{"/etc/nginx/sites-available/*.conf", "/etc/nginx/sites-enabled/*.conf", "/etc/nginx/conf.d/*.conf"} {
		paths, _ := filepath.Glob(g)
		for _, p := range paths {
			ok, err := patchConfFile(p, [][2][]byte{
				{[]byte("proxy_set_header Host $host;"), []byte("proxy_set_header Host $http_host;")},
			})
			if err == nil && ok {
				changed = true
			}
		}
	}
	apacheChanged := false
	for _, g := range []string{"/etc/apache2/sites-available/*.conf", "/etc/apache2/sites-enabled/*.conf"} {
		paths, _ := filepath.Glob(g)
		for _, p := range paths {
			ok, err := ensureApacheForwardHeaders(p)
			if err == nil && ok {
				apacheChanged = true
			}
		}
	}
	if changed && exec.Command("nginx", "-t").Run() == nil {
		_ = exec.Command("nginx", "-s", "reload").Run()
	}
	if apacheChanged {
		_ = exec.Command("systemctl", "reload", "apache2").Run()
	}
}

func (m *Manager) FixAllWebPerms() {
	fixProxyHostHeader()
	ents, err := os.ReadDir(m.HomeRoot)
	if err != nil {
		return
	}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if validate.LinuxUser(name) != nil {
			continue
		}
		home := filepath.Join(m.HomeRoot, name)
		_ = os.Chmod(home, 0711)
		domains := filepath.Join(home, "domains")
		_ = filepath.WalkDir(domains, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				_ = os.Chmod(path, 0755)
			} else {
				_ = os.Chmod(path, 0644)
			}
			return nil
		})
	}
}

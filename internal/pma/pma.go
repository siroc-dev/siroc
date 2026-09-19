//go:build linux

package pma

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

const (
	rootDir  = "/opt/siroc/phpmyadmin"
	tokenDir = "/opt/siroc/pma-signon"
	tmpDir   = "/opt/siroc/pma-tmp"
	listen   = "127.0.0.1:9088"
)

func Installed() (bool, string) {
	if _, err := os.Stat(filepath.Join(rootDir, "index.php")); err != nil {
		return false, ""
	}
	return true, "5.2.2"
}

func Setup() error {
	if ok, _ := Installed(); !ok {
		if err := download(); err != nil {
			return err
		}
	}
	return configure()
}

func Refresh() error {
	if err := download(); err != nil {
		return err
	}
	return configure()
}

func configure() error {
	if err := writeConfig(); err != nil {
		return err
	}
	if err := os.MkdirAll(tokenDir, 0750); err != nil {
		return err
	}
	if err := os.MkdirAll(tmpDir, 0750); err != nil {
		return err
	}
	_ = exec.Command("chown", "-R", "www-data:www-data", rootDir).Run()
	_ = exec.Command("chown", "root:www-data", tokenDir).Run()
	_ = exec.Command("chown", "www-data:www-data", tmpDir).Run()
	if err := writePool(); err != nil {
		return err
	}
	if err := writeNginx(); err != nil {
		return err
	}
	_ = exec.Command("systemctl", "restart", phpFPMService()).Run()
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		return fmt.Errorf("nginx: %s", strings.TrimSpace(string(out)))
	}
	return exec.Command("systemctl", "reload", "nginx").Run()
}

func Signon(req rpc.PMASignonReq) (*rpc.PMASignonResp, error) {
	if ok, _ := Installed(); !ok {
		if err := Setup(); err != nil {
			return nil, err
		}
	} else if err := writeConfig(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(tokenDir, 0750); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.DBUser) == "" || req.Password == "" {
		return nil, fmt.Errorf("database user and password required")
	}
	host := req.Host
	if host == "" {
		host = "127.0.0.1"
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(b)
	payload, _ := json.Marshal(map[string]any{
		"user": req.DBUser,
		"pass": req.Password,
		"host": host,
		"exp":  time.Now().Add(2 * time.Minute).Unix(),
	})
	path := filepath.Join(tokenDir, token+".json")
	if err := os.WriteFile(path, payload, 0640); err != nil {
		return nil, err
	}
	_ = exec.Command("chown", "root:www-data", path).Run()
	return &rpc.PMASignonResp{OK: true, Token: token, URL: "/pma/signon.php?t=" + token}, nil
}

func download() error {
	if err := os.MkdirAll("/opt/siroc", 0755); err != nil {
		return err
	}
	tmp := "/opt/siroc/phpmyadmin.tgz"
	url := "https://files.phpmyadmin.net/phpMyAdmin/5.2.2/phpMyAdmin-5.2.2-all-languages.tar.gz"
	if out, err := exec.Command("curl", "-fsSL", "-o", tmp, url).CombinedOutput(); err != nil {
		return fmt.Errorf("download phpMyAdmin: %s", strings.TrimSpace(string(out)))
	}
	_ = os.RemoveAll(rootDir)
	if out, err := exec.Command("tar", "-xzf", tmp, "-C", "/opt/siroc").CombinedOutput(); err != nil {
		return fmt.Errorf("extract phpMyAdmin: %s", strings.TrimSpace(string(out)))
	}
	matches, _ := filepath.Glob("/opt/siroc/phpMyAdmin-*")
	if len(matches) == 0 {
		return fmt.Errorf("phpMyAdmin extract missing")
	}
	if err := os.Rename(matches[0], rootDir); err != nil {
		return err
	}
	_ = os.Remove(tmp)
	return nil
}

func blowfishSecret() string {
	path := "/opt/siroc/pma-blowfish"
	if b, err := os.ReadFile(path); err == nil {
		s := strings.TrimSpace(string(b))
		if len(s) >= 16 {
			return s
		}
	}
	s := randomHex(16)
	_ = os.WriteFile(path, []byte(s+"\n"), 0600)
	return s
}

func writeConfig() error {
	secret := blowfishSecret()
	cfg := `<?php
$cfg['blowfish_secret'] = '` + secret + `';
$cfg['PmaAbsoluteUri'] = '/pma/';
$cfg['CookieSameSite'] = 'Lax';
$cfg['VersionCheck'] = false;
$i = 1;
$cfg['Servers'][$i]['auth_type'] = 'signon';
$cfg['Servers'][$i]['host'] = '127.0.0.1';
$cfg['Servers'][$i]['compress'] = false;
$cfg['Servers'][$i]['AllowNoPassword'] = false;
$cfg['Servers'][$i]['SignonSession'] = 'SignonSession';
$cfg['Servers'][$i]['SignonURL'] = 'signon.php';
$cfg['Servers'][$i]['SignonCookieParams'] = [
    'lifetime' => 0,
    'path' => '/pma/',
    'domain' => '',
    'secure' => false,
    'httponly' => true,
    'samesite' => 'Lax',
];
$cfg['UploadDir'] = '';
$cfg['SaveDir'] = '';
$cfg['TempDir'] = '` + tmpDir + `';
`
	signon := `<?php
$secure = !empty($_SERVER['HTTP_X_FORWARDED_PROTO']) && strtolower((string)$_SERVER['HTTP_X_FORWARDED_PROTO']) === 'https';
session_name('SignonSession');
session_set_cookie_params([
    'lifetime' => 0,
    'path' => '/pma/',
    'domain' => '',
    'secure' => $secure,
    'httponly' => true,
    'samesite' => 'Lax',
]);
session_start();
$token = preg_replace('/[^a-f0-9]/', '', strtolower((string)($_GET['t'] ?? '')));
$file = '` + tokenDir + `/' . $token . '.json';
if ($token !== '' && is_readable($file)) {
    $data = json_decode((string)file_get_contents($file), true);
    @unlink($file);
    if (is_array($data) && !empty($data['user']) && !empty($data['pass']) && (int)($data['exp'] ?? 0) > time()) {
        $_SESSION['PMA_single_signon_user'] = $data['user'];
        $_SESSION['PMA_single_signon_password'] = $data['pass'];
        $_SESSION['PMA_single_signon_host'] = $data['host'] ?? '127.0.0.1';
        header('Location: index.php');
        exit;
    }
}
http_response_code(403);
echo 'phpMyAdmin sign-on expired. Open phpMyAdmin again from the control panel.';
`
	if err := os.WriteFile(filepath.Join(rootDir, "config.inc.php"), []byte(cfg), 0640); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(rootDir, "signon.php"), []byte(signon), 0644); err != nil {
		return err
	}
	_ = exec.Command("chown", "www-data:www-data", filepath.Join(rootDir, "config.inc.php"), filepath.Join(rootDir, "signon.php")).Run()
	return nil
}

func phpVersion() string {
	for _, v := range []string{"8.4", "8.3", "8.2", "8.1"} {
		if _, err := os.Stat("/etc/php/" + v + "/fpm"); err == nil {
			return v
		}
	}
	return "8.3"
}

func phpFPMService() string {
	return "php" + phpVersion() + "-fpm"
}

func nginxUser() string {
	b, err := os.ReadFile("/etc/nginx/nginx.conf")
	if err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "user") {
				continue
			}
			fields := strings.Fields(strings.TrimSuffix(line, ";"))
			if len(fields) >= 2 && fields[0] == "user" {
				return fields[1]
			}
		}
	}
	return "www-data"
}

func writePool() error {
	v := phpVersion()
	dir := "/etc/php/" + v + "/fpm/pool.d"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	sockUser := nginxUser()
	body := fmt.Sprintf(`[pma]
user = www-data
group = www-data
listen = /run/php/php-pma.sock
listen.owner = %s
listen.group = %s
listen.mode = 0660
pm = ondemand
pm.max_children = 8
pm.process_idle_timeout = 10s
php_admin_value[open_basedir] = %s:%s:%s
php_admin_value[upload_tmp_dir] = %s
php_admin_value[session.save_path] = %s
`, sockUser, sockUser, rootDir, tmpDir, tokenDir, tmpDir, tmpDir)
	return os.WriteFile(filepath.Join(dir, "pma.conf"), []byte(body), 0644)
}

func writeNginx() error {
	body := `server {
    listen 127.0.0.1:9088;
    server_name pma.cp.local;
    root /opt/siroc/phpmyadmin;
    index index.php;
    client_max_body_size 64m;
    location / {
        try_files $uri $uri/ /index.php?$args;
    }
    location ~ \.php$ {
        include snippets/fastcgi-php.conf;
        fastcgi_pass unix:/run/php/php-pma.sock;
    }
    location ~ /\. { deny all; }
}
`
	if err := os.WriteFile("/etc/nginx/sites-available/cp-pma.conf", []byte(body), 0644); err != nil {
		return err
	}
	_ = os.MkdirAll("/etc/nginx/sites-enabled", 0755)
	_ = os.Remove("/etc/nginx/sites-enabled/cp-pma.conf")
	return os.Symlink("/etc/nginx/sites-available/cp-pma.conf", "/etc/nginx/sites-enabled/cp-pma.conf")
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

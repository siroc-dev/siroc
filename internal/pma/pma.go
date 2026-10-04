//go:build linux

package pma

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

const (
	rootDir   = "/opt/siroc/phpmyadmin"
	tokenDir  = "/opt/siroc/pma-signon"
	tmpDir    = "/opt/siroc/pma-tmp"
	listen    = "127.0.0.1:9088"
	fpmListen = "127.0.0.1:9008"
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

func Ensure() error {
	return Setup()
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
	v, err := phpVersion()
	if err != nil {
		return err
	}
	if err := writePool(v); err != nil {
		return err
	}
	if err := writeNginx(); err != nil {
		return err
	}
	if err := restartFPM(v); err != nil {
		return err
	}
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		return fmt.Errorf("nginx: %s", strings.TrimSpace(string(out)))
	}
	if err := exec.Command("systemctl", "reload", "nginx").Run(); err != nil {
		if out, startErr := exec.Command("systemctl", "restart", "nginx").CombinedOutput(); startErr != nil {
			return fmt.Errorf("reload nginx: %s", strings.TrimSpace(string(out)))
		}
	}
	if err := waitTCP(listen, 6*time.Second); err != nil {
		return err
	}
	if err := waitTCP(fpmListen, 4*time.Second); err != nil {
		return fmt.Errorf("phpMyAdmin PHP-FPM is not listening on %s: %w", fpmListen, err)
	}
	return nil
}

func Signon(req rpc.PMASignonReq) (*rpc.PMASignonResp, error) {
	if err := Ensure(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(tokenDir, 0750); err != nil {
		return nil, err
	}
	_ = exec.Command("chown", "www-data:www-data", tokenDir).Run()
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
	if err := exec.Command("chown", "www-data:www-data", path).Run(); err != nil {
		_ = os.Chmod(path, 0644)
	}
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
$cfg['ExecTimeLimit'] = 0;
$cfg['MemoryLimit'] = '512M';
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
        session_write_close();
        header('Location: index.php');
        exit;
    }
}
$reason = '';
if (!empty($_SESSION['PMA_single_signon_error_message'])) {
    $reason = trim(strip_tags((string)$_SESSION['PMA_single_signon_error_message']));
}
http_response_code(403);
if ($reason !== '') {
    echo 'phpMyAdmin could not sign in. ' . htmlspecialchars($reason, ENT_QUOTES, 'UTF-8');
    exit;
}
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

func phpVersion() (string, error) {
	var active, installed []string
	for _, v := range phpVersions {
		if _, err := os.Stat("/etc/php/" + v + "/fpm"); err == nil {
			installed = append(installed, v)
		}
		if exec.Command("systemctl", "is-active", "--quiet", "php"+v+"-fpm").Run() == nil {
			active = append(active, v)
		}
	}
	v := pickPHPVersion(active, installed)
	if v == "" {
		return "", fmt.Errorf("PHP-FPM is not installed. Install PHP from Software first")
	}
	return v, nil
}

func restartFPM(v string) error {
	svc := "php" + v + "-fpm"
	_ = exec.Command("systemctl", "enable", svc).Run()
	if err := exec.Command("systemctl", "restart", svc).Run(); err != nil {
		if out, err2 := exec.Command("systemctl", "start", svc).CombinedOutput(); err2 != nil {
			return fmt.Errorf("start %s: %s", svc, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func waitTCP(addr string, d time.Duration) error {
	deadline := time.Now().Add(d)
	var last error
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return nil
		}
		last = err
		time.Sleep(150 * time.Millisecond)
	}
	if last == nil {
		last = fmt.Errorf("timeout")
	}
	return fmt.Errorf("phpMyAdmin backend %s is not listening: %v", addr, last)
}

func writePool(v string) error {
	dir := "/etc/php/" + v + "/fpm/pool.d"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	body := fmt.Sprintf(`[pma]
user = www-data
group = www-data
listen = %s
listen.allowed_clients = 127.0.0.1
pm = ondemand
pm.max_children = 8
pm.process_idle_timeout = 10s
php_admin_value[open_basedir] = %s:%s:%s:/tmp:/usr/share/php
php_admin_value[upload_tmp_dir] = %s
php_admin_value[session.save_path] = %s
php_admin_value[upload_max_filesize] = 10G
php_admin_value[post_max_size] = 11G
php_admin_value[memory_limit] = 512M
php_admin_value[max_execution_time] = 0
php_admin_value[max_input_time] = -1
request_terminate_timeout = 0
`, fpmListen, rootDir, tmpDir, tokenDir, tmpDir, tmpDir)
	return os.WriteFile(filepath.Join(dir, "pma.conf"), []byte(body), 0644)
}

func writeNginx() error {
	body := `server {
    listen 127.0.0.1:9088;
    server_name pma.cp.local;
    root /opt/siroc/phpmyadmin;
    index index.php;
    client_max_body_size 11g;
    client_body_timeout 3600s;
    location / {
        try_files $uri $uri/ /index.php?$args;
    }
    location ~ \.php$ {
        include fastcgi_params;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        fastcgi_param HTTP_X_FORWARDED_PROTO $http_x_forwarded_proto;
        fastcgi_param HTTPS $http_x_forwarded_proto if_not_empty;
        fastcgi_pass 127.0.0.1:9008;
        fastcgi_read_timeout 21600s;
        fastcgi_send_timeout 21600s;
        fastcgi_request_buffering off;
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

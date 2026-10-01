//go:build linux

package backup

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/users"
	"github.com/siroc-dev/siroc/internal/validate"
)

func Run(req rpc.BackupReq) (*rpc.BackupResp, error) {
	if req.Inspect {
		return inspect(req)
	}
	if req.Restore != "" {
		return restore(req)
	}
	if err := validate.LinuxUser(req.Username); err != nil {
		return nil, err
	}
	home := filepath.Join("/home", req.Username)
	if st, err := os.Stat(home); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("home directory not found")
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	dir := filepath.Join("/var/backups/siroc", req.Username)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, err
	}
	stage := filepath.Join("/var/backups/siroc", ".work", req.Username+"-"+stamp)
	meta := filepath.Join(stage, "siroc-meta")
	if err := os.MkdirAll(filepath.Join(meta, "dumps"), 0750); err != nil {
		return nil, err
	}
	defer os.RemoveAll(stage)
	man := req.Manifest
	if man == nil {
		man = &rpc.BackupManifest{Version: 1, Username: req.Username}
	}
	man.Version = 1
	man.Username = req.Username
	if man.Created == "" {
		man.Created = time.Now().UTC().Format(time.RFC3339)
	}
	if man.Account.Username == "" {
		man.Account.Username = req.Username
	}
	raw, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(meta, "manifest.json"), raw, 0640); err != nil {
		return nil, err
	}
	if req.IncludeDB {
		names := req.Databases
		if len(names) == 0 {
			for _, d := range man.Databases {
				if d.DBName != "" {
					names = append(names, d.DBName)
				}
			}
		}
		if err := dumpDatabases(filepath.Join(meta, "dumps"), names); err != nil {
			return nil, err
		}
	}
	writeCrontab(req.Username, filepath.Join(meta, "crontab"))
	copySiteCerts(man.Sites, filepath.Join(meta, "ssl"))
	archive := filepath.Join(dir, req.Username+"-"+stamp+".tar.gz")
	args := []string{"-czf", archive, "-C", "/home", req.Username, "-C", stage, "siroc-meta"}
	if out, err := exec.Command("tar", args...).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("archive: %s", strings.TrimSpace(string(out)))
	}
	fi, err := os.Stat(archive)
	if err != nil {
		return nil, err
	}
	resp := &rpc.BackupResp{OK: true, Path: archive, Size: fi.Size(), Manifest: man, Message: "saved " + filepath.Base(archive)}
	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	if kind == "" {
		kind = "local"
	}
	switch kind {
	case "local":
		if req.LocalDir != "" {
			if err := os.MkdirAll(req.LocalDir, 0750); err != nil {
				return nil, err
			}
			dest := filepath.Join(req.LocalDir, filepath.Base(archive))
			if out, err := exec.Command("cp", "-a", archive, dest).CombinedOutput(); err != nil {
				return nil, fmt.Errorf("copy local: %s", strings.TrimSpace(string(out)))
			}
			resp.Remote = dest
		}
	case "ftp":
		remote, err := uploadFTP(req, archive)
		if err != nil {
			return nil, err
		}
		resp.Remote = remote
	case "s3":
		remote, err := uploadS3(req, archive)
		if err != nil {
			return nil, err
		}
		resp.Remote = remote
	default:
		return nil, fmt.Errorf("unknown backup kind %q", kind)
	}
	return resp, nil
}

func inspect(req rpc.BackupReq) (*rpc.BackupResp, error) {
	src := strings.TrimSpace(req.Restore)
	if !RestorePathOK(src, req.Username) {
		return nil, fmt.Errorf("restore path is not allowed")
	}
	man, err := readManifest(src)
	if err != nil {
		return nil, err
	}
	return &rpc.BackupResp{OK: true, Path: src, Manifest: man, Message: "archive for " + man.Username}, nil
}

func restore(req rpc.BackupReq) (*rpc.BackupResp, error) {
	src := strings.TrimSpace(req.Restore)
	if !RestorePathOK(src, req.Username) {
		return nil, fmt.Errorf("restore path is not allowed")
	}
	tmp := filepath.Join("/var/backups/siroc", ".restore", fmt.Sprintf("%d", time.Now().UnixNano()))
	if err := os.MkdirAll(tmp, 0750); err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	if out, err := exec.Command("tar", "-xzf", src, "-C", tmp).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("restore: %s", strings.TrimSpace(string(out)))
	}
	man, err := loadExtractedManifest(tmp)
	if err != nil {
		man = req.Manifest
	}
	if man == nil {
		man = &rpc.BackupManifest{Version: 1, Username: req.Username}
	}
	username := strings.TrimSpace(man.Username)
	if username == "" {
		username = strings.TrimSpace(req.Username)
	}
	if err := validate.LinuxUser(username); err != nil {
		return nil, err
	}
	man.Username = username
	if man.Account.Username == "" {
		man.Account.Username = username
	}
	pass := strings.TrimSpace(man.Account.Password)
	_, existed := user.Lookup(username)
	created := existed != nil
	homeSrc := filepath.Join(tmp, username)
	home := filepath.Join("/home", username)
	if st, err := os.Stat(homeSrc); err == nil && st.IsDir() {
		if err := os.MkdirAll(filepath.Dir(home), 0755); err != nil {
			return nil, err
		}
		if _, err := os.Stat(home); err != nil {
			if out, cpErr := exec.Command("cp", "-a", homeSrc, home).CombinedOutput(); cpErr != nil {
				return nil, fmt.Errorf("restore home: %s", strings.TrimSpace(string(out)))
			}
		} else if out, cpErr := exec.Command("cp", "-a", homeSrc+"/.", home+"/").CombinedOutput(); cpErr != nil {
			return nil, fmt.Errorf("restore home: %s", strings.TrimSpace(string(out)))
		}
	}
	um := &users.Manager{HomeRoot: "/home"}
	if _, err := um.Ensure(username, 0, 0, pass); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	_ = exec.Command("chown", "-R", username+":"+username, home).Run()
	_ = os.Chmod(home, 0711)
	if err := importDumps(filepath.Join(tmp, "siroc-meta", "dumps"), man.Databases); err != nil {
		return nil, err
	}
	restoreCrontab(username, filepath.Join(tmp, "siroc-meta", "crontab"))
	restoreSiteCerts(filepath.Join(tmp, "siroc-meta", "ssl"))
	msg := "restored " + username + " from " + filepath.Base(src)
	if created {
		msg = "created user " + username + " and restored from " + filepath.Base(src)
	}
	return &rpc.BackupResp{OK: true, Path: src, CreatedUser: created, Manifest: man, Message: msg}, nil
}

func dumpDatabases(dir string, names []string) error {
	if len(names) == 0 {
		return nil
	}
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	dumpBin := "mysqldump"
	if _, err := exec.LookPath("mariadb-dump"); err == nil {
		dumpBin = "mariadb-dump"
	}
	for _, name := range names {
		if err := validate.DBIdent(name); err != nil {
			return err
		}
		var dumpOut []byte
		var dumpErr error
		for _, extra := range [][]string{{"--skip-ssl"}, {"--ssl-mode=DISABLED"}, nil} {
			dargs := append([]string{"--single-transaction", "--routines"}, extra...)
			dargs = append(dargs, "--databases", name)
			dumpOut, dumpErr = exec.Command(dumpBin, dargs...).CombinedOutput()
			if dumpErr == nil {
				break
			}
		}
		if dumpErr != nil {
			return fmt.Errorf("mysqldump %s: %s", name, strings.TrimSpace(string(dumpOut)))
		}
		if err := os.WriteFile(filepath.Join(dir, name+".sql"), dumpOut, 0640); err != nil {
			return err
		}
	}
	return nil
}

func importDumps(dir string, dbs []rpc.BackupDatabase) error {
	bin := "mysql"
	if _, err := exec.LookPath("mariadb"); err == nil {
		bin = "mariadb"
	}
	if exec.Command(bin, "--version").Run() != nil {
		if len(dbs) == 0 {
			return nil
		}
		return fmt.Errorf("MySQL/MariaDB is not installed")
	}
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return err
		}
		cmd := exec.Command(bin)
		cmd.Stdin = bytes.NewReader(b)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("import %s: %s", e.Name(), strings.TrimSpace(string(out)))
		}
	}
	for _, d := range dbs {
		if err := validate.DBIdent(d.DBName); err != nil {
			continue
		}
		if err := validate.DBIdent(d.DBUser); err != nil || d.Password == "" {
			continue
		}
		sql := strings.Join([]string{
			fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`;", d.DBName),
			fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'localhost' IDENTIFIED BY '%s';", escapeSQL(d.DBUser), escapeSQL(d.Password)),
			fmt.Sprintf("ALTER USER '%s'@'localhost' IDENTIFIED BY '%s';", escapeSQL(d.DBUser), escapeSQL(d.Password)),
			fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'localhost';", d.DBName, escapeSQL(d.DBUser)),
			"FLUSH PRIVILEGES;",
		}, "\n")
		cmd := exec.Command(bin)
		cmd.Stdin = strings.NewReader(sql)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("restore database user %s: %s", d.DBUser, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func escapeSQL(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `''`)
	return s
}

func writeCrontab(username, dest string) {
	out, err := exec.Command("crontab", "-u", username, "-l").CombinedOutput()
	if err != nil {
		return
	}
	_ = os.WriteFile(dest, out, 0640)
}

func restoreCrontab(username, src string) {
	b, err := os.ReadFile(src)
	if err != nil || len(bytes.TrimSpace(b)) == 0 {
		return
	}
	cmd := exec.Command("crontab", "-u", username, "-")
	cmd.Stdin = bytes.NewReader(b)
	_ = cmd.Run()
}

func copySiteCerts(sites []rpc.BackupSite, dest string) {
	for _, st := range sites {
		domain := strings.TrimSpace(st.Domain)
		if domain == "" {
			continue
		}
		live := filepath.Join("/etc/letsencrypt/live", domain)
		if _, err := os.Stat(filepath.Join(live, "fullchain.pem")); err != nil {
			continue
		}
		out := filepath.Join(dest, domain)
		_ = os.MkdirAll(out, 0750)
		for _, name := range []string{"fullchain.pem", "privkey.pem", "chain.pem", "cert.pem"} {
			_ = exec.Command("cp", "-a", filepath.Join(live, name), filepath.Join(out, name)).Run()
		}
	}
}

func restoreSiteCerts(src string) {
	ents, err := os.ReadDir(src)
	if err != nil {
		return
	}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		domain := e.Name()
		if validate.Domain(domain) != nil {
			continue
		}
		live := filepath.Join("/etc/letsencrypt/live", domain)
		_ = os.MkdirAll(live, 0755)
		_ = exec.Command("cp", "-a", filepath.Join(src, domain)+"/.", live+"/").Run()
	}
}

func readManifest(archive string) (*rpc.BackupManifest, error) {
	cmd := exec.Command("tar", "-xOf", archive, "siroc-meta/manifest.json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("archive has no user metadata")
	}
	var man rpc.BackupManifest
	if err := json.Unmarshal(out, &man); err != nil {
		return nil, fmt.Errorf("invalid backup manifest")
	}
	if strings.TrimSpace(man.Username) == "" {
		return nil, fmt.Errorf("backup manifest is missing a username")
	}
	return &man, nil
}

func loadExtractedManifest(root string) (*rpc.BackupManifest, error) {
	b, err := os.ReadFile(filepath.Join(root, "siroc-meta", "manifest.json"))
	if err != nil {
		return nil, err
	}
	var man rpc.BackupManifest
	if err := json.Unmarshal(b, &man); err != nil {
		return nil, err
	}
	return &man, nil
}

func uploadFTP(req rpc.BackupReq, local string) (string, error) {
	host := strings.TrimSpace(req.Host)
	if host == "" {
		return "", fmt.Errorf("FTP host required")
	}
	port := req.Port
	if port <= 0 {
		port = 21
	}
	remotePath := strings.TrimSuffix(req.Path, "/") + "/" + filepath.Base(local)
	if !strings.HasPrefix(remotePath, "/") {
		remotePath = "/" + remotePath
	}
	u := url.URL{
		Scheme: "ftp",
		Host:   fmt.Sprintf("%s:%d", host, port),
		Path:   remotePath,
	}
	if req.User != "" {
		u.User = url.UserPassword(req.User, req.Password)
	}
	cmd := exec.Command("curl", "-fsS", "--connect-timeout", "20", "--max-time", "600", "-T", local, u.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ftp upload: %s", strings.TrimSpace(string(out)))
	}
	return u.Scheme + "://" + host + remotePath, nil
}

func uploadS3(req rpc.BackupReq, local string) (string, error) {
	if req.Bucket == "" || req.AccessKey == "" || req.SecretKey == "" {
		return "", fmt.Errorf("S3 bucket, access key, and secret key are required")
	}
	key := strings.Trim(req.Prefix, "/")
	if key != "" {
		key += "/"
	}
	key += filepath.Base(local)
	endpoint := strings.TrimSpace(req.Endpoint)
	endpoint = strings.TrimPrefix(endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "http://")
	if endpoint == "" {
		endpoint = "s3.amazonaws.com"
	}
	region := req.Region
	if region == "" {
		region = "us-east-1"
	}
	scheme := "https"
	if !req.UseSSL && strings.HasPrefix(strings.ToLower(req.Endpoint), "http://") {
		scheme = "http"
	}
	body, err := os.ReadFile(local)
	if err != nil {
		return "", err
	}
	host := endpoint
	path := "/" + req.Bucket + "/" + key
	uri := scheme + "://" + host + path
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	payloadHash := sha256Hex(body)
	canonicalHeaders := "host:" + host + "\n" + "x-amz-content-sha256:" + payloadHash + "\n" + "x-amz-date:" + amzDate + "\n"
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonical := strings.Join([]string{"PUT", path, "", canonicalHeaders, signedHeaders, payloadHash}, "\n")
	scope := dateStamp + "/" + region + "/s3/aws4_request"
	stringToSign := strings.Join([]string{"AWS4-HMAC-SHA256", amzDate, scope, sha256Hex([]byte(canonical))}, "\n")
	signing := hmacSHA256([]byte("AWS4"+req.SecretKey), []byte(dateStamp))
	signing = hmacSHA256(signing, []byte(region))
	signing = hmacSHA256(signing, []byte("s3"))
	signing = hmacSHA256(signing, []byte("aws4_request"))
	sig := hex.EncodeToString(hmacSHA256(signing, []byte(stringToSign)))
	auth := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s", req.AccessKey, scope, signedHeaders, sig)
	httpReq, err := http.NewRequest(http.MethodPut, uri, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Authorization", auth)
	httpReq.Header.Set("x-amz-date", amzDate)
	httpReq.Header.Set("x-amz-content-sha256", payloadHash)
	httpReq.Header.Set("Content-Type", "application/gzip")
	client := &http.Client{Timeout: 10 * time.Minute}
	res, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return "", fmt.Errorf("s3 upload %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	return "s3://" + req.Bucket + "/" + key, nil
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func hmacSHA256(key, data []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(data)
	return m.Sum(nil)
}

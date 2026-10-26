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
	"log"
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
	if req.Retain > 0 {
		pruneDir(dir, req.Username, req.Retain)
		if kind == "local" && req.LocalDir != "" && filepath.Clean(req.LocalDir) != filepath.Clean(dir) {
			pruneDir(req.LocalDir, req.Username, req.Retain)
		}
	}
	return resp, nil
}

func pruneDir(dir, username string, keep int) {
	matches, err := filepath.Glob(filepath.Join(dir, username+"-*.tar.gz"))
	if err != nil || len(matches) == 0 {
		return
	}
	names := make([]string, 0, len(matches))
	for _, p := range matches {
		names = append(names, filepath.Base(p))
	}
	for _, name := range PruneNames(names, keep) {
		_ = os.Remove(filepath.Join(dir, name))
	}
}

func inspect(req rpc.BackupReq) (*rpc.BackupResp, error) {
	src := strings.TrimSpace(req.Restore)
	if !RestorePathOK(src, req.Username) {
		log.Printf("restore: path not allowed: %s", src)
		return nil, fmt.Errorf("restore path is not allowed: %s", src)
	}
	if man, err := readManifest(src); err == nil {
		return &rpc.BackupResp{OK: true, Path: src, Manifest: man, Message: "archive for " + man.Username}, nil
	}
	tmp, err := extractToTemp(src)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	resp, err := describeExtract(tmp)
	if err != nil {
		return nil, err
	}
	resp.Path = src
	return resp, nil
}

func restore(req rpc.BackupReq) (*rpc.BackupResp, error) {
	src := strings.TrimSpace(req.Restore)
	if !RestorePathOK(src, req.Username) {
		log.Printf("restore: path not allowed: %s", src)
		return nil, fmt.Errorf("restore path is not allowed: %s", src)
	}
	tmp, err := extractToTemp(src)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	if _, err := loadExtractedManifest(tmp); err == nil {
		return restoreExtracted(tmp, src, req)
	}
	if _, err := ParseDirectAdmin(tmp); err == nil {
		return restoreExtracted(tmp, src, req)
	}
	nested := NestedUserArchives(tmp)
	if len(nested) == 0 {
		return nil, fmt.Errorf("archive has no user metadata")
	}
	if len(nested) > 50 {
		return nil, fmt.Errorf("this DirectAdmin backup has %d accounts; restore up to 50 at a time", len(nested))
	}
	var mans []*rpc.BackupManifest
	var names []string
	created := false
	for i, arc := range nested {
		sub := filepath.Join(tmp, fmt.Sprintf("user-%d", i))
		if err := extractArchive(arc, sub); err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(arc), err)
		}
		if _, err := ParseDirectAdmin(sub); err != nil {
			continue
		}
		one, err := restoreExtracted(sub, src, req)
		if err != nil {
			if req.Username != "" && strings.Contains(err.Error(), "archive is for ") {
				continue
			}
			if len(req.Domains) > 0 && strings.Contains(err.Error(), "none of the selected domains") {
				continue
			}
			return nil, fmt.Errorf("%s: %w", filepath.Base(arc), err)
		}
		if one.Manifest != nil {
			mans = append(mans, one.Manifest)
			names = append(names, one.Manifest.Username)
		}
		created = created || one.CreatedUser
	}
	if len(mans) == 0 {
		if len(req.Domains) > 0 {
			return nil, fmt.Errorf("backup has none of the selected domains")
		}
		if req.Username != "" {
			return nil, fmt.Errorf("no DirectAdmin account named %s in this backup", req.Username)
		}
		return nil, fmt.Errorf("archive has no user metadata")
	}
	msg := "restored " + strings.Join(names, ", ") + " from DirectAdmin. Mail, DNS, and certificates were not imported. Set each new account password under Accounts."
	return &rpc.BackupResp{OK: true, Path: src, CreatedUser: created, Manifest: mans[0], Manifests: mans, Message: msg}, nil
}

func RestoreDirectAdminDomains(archive string, domains []string, php string) (*rpc.BackupResp, error) {
	return restore(rpc.BackupReq{Restore: archive, Domains: domains, PHPVersion: php})
}

func SaveUpload(name string, r io.Reader) (string, error) {
	safe, err := RestoreArchiveName(name)
	if err != nil {
		return "", err
	}
	dir := "/var/backups/siroc/incoming"
	if err := os.MkdirAll(dir, 0750); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, time.Now().UTC().Format("20060102-150405")+"-"+safe)
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
	if err != nil {
		return "", err
	}
	n, copyErr := io.Copy(f, io.LimitReader(r, MaxRestoreUpload+1))
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil || n > MaxRestoreUpload || n == 0 {
		_ = os.Remove(dest)
		if n > MaxRestoreUpload {
			return "", fmt.Errorf("upload too large (max 10GB)")
		}
		if n == 0 && copyErr == nil && closeErr == nil {
			return "", fmt.Errorf("file is empty")
		}
		if copyErr != nil {
			return "", copyErr
		}
		return "", closeErr
	}
	return dest, nil
}

func describeExtract(tmp string) (*rpc.BackupResp, error) {
	if man, err := loadExtractedManifest(tmp); err == nil && strings.TrimSpace(man.Username) != "" {
		return &rpc.BackupResp{OK: true, Manifest: man, Message: "archive for " + man.Username}, nil
	}
	if plan, err := ParseDirectAdmin(tmp); err == nil {
		return &rpc.BackupResp{OK: true, Manifest: plan.Manifest, Message: directAdminMessage(plan.Manifest.Username, len(plan.Manifest.Sites), len(plan.Manifest.Databases), len(plan.Manifest.FTP), plan.ResetDBPasswords, false)}, nil
	}
	nested := NestedUserArchives(tmp)
	if len(nested) == 0 {
		return nil, fmt.Errorf("archive has no user metadata")
	}
	return &rpc.BackupResp{OK: true, Message: fmt.Sprintf("DirectAdmin backup with %d accounts", len(nested))}, nil
}

func restoreExtracted(tmp, src string, req rpc.BackupReq) (*rpc.BackupResp, error) {
	var plan *directAdminPlan
	man, err := loadExtractedManifest(tmp)
	if err != nil || man == nil || strings.TrimSpace(man.Username) == "" {
		plan, err = ParseDirectAdmin(tmp)
		if err != nil {
			if req.Manifest != nil && strings.TrimSpace(req.Manifest.Username) != "" {
				man = req.Manifest
			} else if req.Username != "" {
				man = &rpc.BackupManifest{Version: 1, Username: req.Username}
			} else {
				return nil, fmt.Errorf("archive has no user metadata")
			}
		} else {
			man = plan.Manifest
		}
	}
	if plan != nil && len(req.Domains) > 0 {
		if err := plan.selectDomains(req.Domains, req.PHPVersion); err != nil {
			return nil, err
		}
		man = plan.Manifest
	}
	username := strings.TrimSpace(man.Username)
	if username == "" {
		username = strings.TrimSpace(req.Username)
	}
	if err := validate.LinuxUser(username); err != nil {
		return nil, err
	}
	if req.Username != "" && req.Username != username {
		return nil, fmt.Errorf("archive is for %s", username)
	}
	man.Username = username
	if man.Account.Username == "" {
		man.Account.Username = username
	}
	pass := strings.TrimSpace(man.Account.Password)
	_, existed := user.Lookup(username)
	created := existed != nil
	home := filepath.Join("/home", username)
	if plan == nil {
		homeSrc := filepath.Join(tmp, username)
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
	}
	um := &users.Manager{HomeRoot: "/home"}
	if _, err := um.Ensure(username, 0, 0, pass); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	if plan != nil {
		if err := applyDirectAdmin(plan, home, tmp); err != nil {
			return nil, err
		}
	}
	_ = exec.Command("chown", "-R", username+":"+username, home).Run()
	_ = os.Chmod(home, 0711)
	if err := importDumps(filepath.Join(tmp, "siroc-meta", "dumps"), man.Databases); err != nil {
		return nil, err
	}
	if plan == nil {
		restoreCrontab(username, filepath.Join(tmp, "siroc-meta", "crontab"))
		restoreSiteCerts(filepath.Join(tmp, "siroc-meta", "ssl"))
	}
	msg := "restored " + username + " from " + filepath.Base(src)
	if created {
		msg = "created user " + username + " and restored from " + filepath.Base(src)
	}
	if plan != nil {
		msg = directAdminMessage(username, len(man.Sites), len(man.Databases), len(man.FTP), plan.ResetDBPasswords, created)
	}
	return &rpc.BackupResp{OK: true, Path: src, CreatedUser: created, Manifest: man, Manifests: []*rpc.BackupManifest{man}, Message: msg}, nil
}

func applyDirectAdmin(plan *directAdminPlan, home, tmp string) error {
	for _, copy := range plan.Copies {
		dest, err := safeHomeDest(home, copy.Rel)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(dest, 0755); err != nil {
			return err
		}
		if out, err := exec.Command("cp", "-a", copy.Src+"/.", dest+"/").CombinedOutput(); err != nil {
			return fmt.Errorf("restore website files: %s", strings.TrimSpace(string(out)))
		}
		for _, skip := range []string{"stats", "logs", "awstats"} {
			_ = os.RemoveAll(filepath.Join(dest, skip))
		}
		_ = filepath.Walk(dest, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil {
				return nil
			}
			if info.IsDir() {
				_ = os.Chmod(path, 0755)
			} else if info.Mode().IsRegular() {
				_ = os.Chmod(path, 0644)
			}
			return nil
		})
	}
	if len(plan.Dumps) == 0 {
		return nil
	}
	dir := filepath.Join(tmp, "siroc-meta", "dumps")
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	for _, dump := range plan.Dumps {
		if err := validate.DBIdent(dump.Name); err != nil {
			continue
		}
		if out, err := exec.Command("cp", "-a", dump.Src, filepath.Join(dir, dump.Name+".sql")).CombinedOutput(); err != nil {
			return fmt.Errorf("stage database dump: %s", strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func safeHomeDest(home, rel string) (string, error) {
	rel = filepath.Clean(strings.ReplaceAll(rel, "\\", "/"))
	if rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("invalid restore path")
	}
	dest := filepath.Join(home, filepath.FromSlash(rel))
	home = filepath.Clean(home)
	if dest != home && !strings.HasPrefix(dest, home+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid restore path")
	}
	return dest, nil
}

func extractToTemp(src string) (string, error) {
	if err := os.MkdirAll("/home/tmp", 0750); err != nil {
		return "", err
	}
	tmp := filepath.Join("/home/tmp", fmt.Sprintf("restore-%d", time.Now().UnixNano()))
	if err := extractArchive(src, tmp); err != nil {
		os.RemoveAll(tmp)
		return "", err
	}
	return tmp, nil
}

func extractArchive(src, dest string) error {
	if err := os.MkdirAll(dest, 0750); err != nil {
		return err
	}
	lower := strings.ToLower(src)
	args := []string{"-xf", src, "-C", dest}
	switch {
	case strings.HasSuffix(lower, ".tar.zst"), strings.HasSuffix(lower, ".tzst"):
		if _, err := exec.LookPath("zstd"); err != nil {
			return fmt.Errorf("install zstd to restore a DirectAdmin .tar.zst backup")
		}
		args = []string{"-I", "zstd", "-xf", src, "-C", dest}
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		args = []string{"-xzf", src, "-C", dest}
	}
	if out, err := exec.Command("tar", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("restore: %s", strings.TrimSpace(string(out)))
	}
	return nil
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
		target := strings.TrimSuffix(e.Name(), ".sql")
		args := []string{}
		if err := validate.DBIdent(target); err == nil {
			create := exec.Command(bin)
			create.Stdin = strings.NewReader(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`;", target))
			if out, err := create.CombinedOutput(); err != nil {
				return fmt.Errorf("create database %s: %s", target, strings.TrimSpace(string(out)))
			}
			args = []string{target}
		}
		f, err := os.Open(filepath.Join(dir, e.Name()))
		if err != nil {
			return err
		}
		cmd := exec.Command(bin, args...)
		cmd.Stdin = f
		out, runErr := cmd.CombinedOutput()
		f.Close()
		if runErr != nil {
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
		var userSQL []string
		userSQL = append(userSQL, fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`;", d.DBName))
		for _, host := range []string{"localhost", "127.0.0.1"} {
			userSQL = append(userSQL,
				fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%s' IDENTIFIED BY '%s';", escapeSQL(d.DBUser), host, escapeSQL(d.Password)),
				fmt.Sprintf("ALTER USER '%s'@'%s' IDENTIFIED BY '%s';", escapeSQL(d.DBUser), host, escapeSQL(d.Password)),
				fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'%s';", d.DBName, escapeSQL(d.DBUser), host),
			)
		}
		userSQL = append(userSQL, "FLUSH PRIVILEGES;")
		sql := strings.Join(userSQL, "\n")
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

//go:build linux

package backup

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/validate"
)

func Run(req rpc.BackupReq) (*rpc.BackupResp, error) {
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
	archive := filepath.Join(dir, req.Username+"-"+stamp+".tar.gz")
	args := []string{"-czf", archive, "-C", "/home", req.Username}
	if req.IncludeDB && len(req.Databases) > 0 {
		sqlPath := filepath.Join(dir, req.Username+"-"+stamp+".sql")
		dumpBin := "mysqldump"
		if _, err := exec.LookPath("mariadb-dump"); err == nil {
			dumpBin = "mariadb-dump"
		}
		var dumpOut []byte
		var dumpErr error
		for _, extra := range [][]string{{"--skip-ssl"}, {"--ssl-mode=DISABLED"}, nil} {
			dargs := append([]string{"--single-transaction", "--routines"}, extra...)
			dargs = append(dargs, "--databases")
			dargs = append(dargs, req.Databases...)
			cmd := exec.Command(dumpBin, dargs...)
			dumpOut, dumpErr = cmd.CombinedOutput()
			if dumpErr == nil {
				break
			}
		}
		if dumpErr != nil {
			return nil, fmt.Errorf("mysqldump: %s", strings.TrimSpace(string(dumpOut)))
		}
		if err := os.WriteFile(sqlPath, dumpOut, 0640); err != nil {
			return nil, err
		}
		defer os.Remove(sqlPath)
		args = append(args, "-C", dir, filepath.Base(sqlPath))
	}
	if out, err := exec.Command("tar", args...).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("archive: %s", strings.TrimSpace(string(out)))
	}
	fi, err := os.Stat(archive)
	if err != nil {
		return nil, err
	}
	resp := &rpc.BackupResp{OK: true, Path: archive, Size: fi.Size()}
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

func restore(req rpc.BackupReq) (*rpc.BackupResp, error) {
	if err := validate.LinuxUser(req.Username); err != nil {
		return nil, err
	}
	src := strings.TrimSpace(req.Restore)
	if src == "" || strings.Contains(src, "..") {
		return nil, fmt.Errorf("invalid restore path")
	}
	if !strings.HasPrefix(src, "/var/backups/siroc/") && !strings.HasPrefix(src, "/home/"+req.Username+"/") {
		return nil, fmt.Errorf("restore path is not allowed")
	}
	home := filepath.Join("/home", req.Username)
	cmd := exec.Command("tar", "-xzf", src, "-C", "/home")
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("restore: %s", strings.TrimSpace(string(out)))
	}
	_ = exec.Command("chown", "-R", req.Username+":"+req.Username, home).Run()
	return &rpc.BackupResp{OK: true, Path: src, Message: "restored into " + home}, nil
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

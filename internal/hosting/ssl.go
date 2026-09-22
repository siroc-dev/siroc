//go:build linux

package hosting

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/validate"
)

const (
	leStaging = "https://acme-staging-v02.api.letsencrypt.org/directory"
)

const leWebroot = "/var/www/letsencrypt"

func liveDir(domain string) string {
	return filepath.Join("/etc/letsencrypt/live", domain)
}

func liveCert(domain string) string {
	return filepath.Join(liveDir(domain), "fullchain.pem")
}

func liveKey(domain string) string {
	return filepath.Join(liveDir(domain), "privkey.pem")
}

func localSSLDir(domain string) string {
	return filepath.Join("/etc/siroc/ssl", domain)
}

func localCert(domain string) string {
	return filepath.Join(localSSLDir(domain), "fullchain.pem")
}

func localKey(domain string) string {
	return filepath.Join(localSSLDir(domain), "privkey.pem")
}

func fileOK(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Size() > 0
}

func leExists(domain string) bool {
	return fileOK(liveCert(domain)) && fileOK(liveKey(domain))
}

func localExists(domain string) bool {
	return fileOK(localCert(domain)) && fileOK(localKey(domain))
}

func resolveCerts(domain, prefer string) (kind, cert, key string) {
	if prefer != "local" && leExists(domain) {
		return "letsencrypt", liveCert(domain), liveKey(domain)
	}
	if localExists(domain) {
		return "local", localCert(domain), localKey(domain)
	}
	if leExists(domain) {
		return "letsencrypt", liveCert(domain), liveKey(domain)
	}
	return "", "", ""
}

func ensureLocalCert(domain string, aliases []string) error {
	if err := validate.Domain(domain); err != nil {
		return err
	}
	dir := localSSLDir(domain)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	cert := localCert(domain)
	key := localKey(domain)
	if localExists(domain) {
		return nil
	}
	names := []string{domain}
	for _, a := range aliases {
		a = strings.ToLower(strings.TrimSpace(a))
		if a != "" && a != domain {
			names = append(names, a)
		}
	}
	san := "DNS:" + strings.Join(names, ",DNS:")
	cnf := filepath.Join(dir, "openssl.cnf")
	cfg := `[req]
distinguished_name = req_distinguished_name
x509_extensions = v3_req
prompt = no
[req_distinguished_name]
CN = ` + domain + `
[v3_req]
subjectAltName = ` + san + `
basicConstraints = CA:FALSE
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
`
	if err := os.WriteFile(cnf, []byte(cfg), 0644); err != nil {
		return err
	}
	cmd := exec.Command("openssl", "req", "-x509", "-newkey", "rsa:2048", "-sha256", "-days", "3650", "-nodes",
		"-keyout", key, "-out", cert, "-config", cnf)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("local ssl: %s: %w", strings.TrimSpace(string(out)), err)
	}
	_ = os.Chmod(cert, 0644)
	_ = os.Chmod(key, 0644)
	_ = os.Remove(cnf)
	return nil
}

func certExpiryPath(cert string) string {
	out, err := exec.Command("openssl", "x509", "-enddate", "-noout", "-in", cert).CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSpace(string(out)), "notAfter=")
}

func (m *Manager) IssueSSL(req rpc.SiteSSLReq) (*rpc.SiteSSLResp, error) {
	if err := validate.LinuxUser(req.Username); err != nil {
		return nil, err
	}
	aliases, err := validate.DomainAliases(req.Domain, req.Aliases)
	if err != nil {
		return nil, err
	}
	if err := validate.Email(req.Email); err != nil {
		return nil, err
	}
	if _, err := exec.LookPath("certbot"); err != nil {
		return nil, fmt.Errorf("certbot is not installed; install it from Software")
	}
	dirURL, custom, err := acmeDirectory(req)
	if err != nil {
		return nil, err
	}
	names := append([]string{req.Domain}, aliases...)
	if !custom {
		for _, n := range names {
			if strings.HasPrefix(n, "*.") {
				continue
			}
			if !publicACMEName(n) {
				return nil, fmt.Errorf("Let's Encrypt cannot issue for %q: domain name does not end with a valid public suffix (TLD). Use local HTTPS for lab domains (.test, .local), or set a custom ACME server under Websites → Let's Encrypt", n)
			}
		}
	}
	if err := os.MkdirAll(filepath.Join(leWebroot, ".well-known/acme-challenge"), 0755); err != nil {
		return nil, err
	}
	args := []string{
		"certonly", "--webroot", "-w", leWebroot,
		"--cert-name", req.Domain,
		"--non-interactive", "--agree-tos", "--keep-until-expiring", "--expand",
		"--preferred-challenges", "http",
	}
	email := strings.TrimSpace(req.Email)
	if email != "" {
		args = append(args, "-m", email)
	} else {
		args = append(args, "--register-unsafely-without-email")
	}
	if dirURL != "" {
		args = append(args, "--server", dirURL)
	}
	if req.NoVerify && custom {
		args = append(args, "--no-verify-ssl")
	}
	keyType := strings.ToLower(strings.TrimSpace(req.KeyType))
	switch keyType {
	case "", "ecdsa", "ecdsa-p256":
		args = append(args, "--key-type", "ecdsa", "--elliptic-curve", "secp256r1")
	case "ecdsa-p384":
		args = append(args, "--key-type", "ecdsa", "--elliptic-curve", "secp384r1")
	case "rsa":
		size := req.RSAKeySize
		if size != 4096 {
			size = 2048
		}
		args = append(args, "--key-type", "rsa", "--rsa-key-size", strconv.Itoa(size))
	default:
		return nil, fmt.Errorf("invalid ACME key type")
	}
	if kid := strings.TrimSpace(req.EABKID); kid != "" {
		hmac := strings.TrimSpace(req.EABHMAC)
		if hmac == "" {
			return nil, fmt.Errorf("EAB HMAC key is required when EAB Key ID is set")
		}
		args = append(args, "--eab-kid", kid, "--eab-hmac-key", hmac)
	}
	args = append(args, "-d", req.Domain)
	for _, a := range aliases {
		if strings.HasPrefix(a, "*.") {
			continue
		}
		args = append(args, "-d", a)
	}
	out, err := exec.Command("certbot", args...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("Let's Encrypt: %s", tailOut(msg))
	}
	if !leExists(req.Domain) {
		return nil, fmt.Errorf("Let's Encrypt finished but certificate is missing")
	}
	return &rpc.SiteSSLResp{OK: true, Kind: "letsencrypt", Expiry: certExpiryPath(liveCert(req.Domain)), Cert: liveCert(req.Domain)}, nil
}

func (m *Manager) RegisterLEAccount(req rpc.LEAccountReq) (*rpc.LEAccountResp, error) {
	if _, err := exec.LookPath("certbot"); err != nil {
		return &rpc.LEAccountResp{CertbotInstalled: false, Message: "certbot is not installed; install it from Software"}, fmt.Errorf("certbot is not installed; install it from Software")
	}
	if err := validate.Email(req.Email); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Email) == "" {
		return nil, fmt.Errorf("Let's Encrypt account email is required")
	}
	flags, err := certbotACMEFlags(req.Server, req.Directory, req.NoVerify, req.EABKID, req.EABHMAC)
	if err != nil {
		return nil, err
	}
	st := showLEAccount(flags)
	if !st.Registered {
		args := append([]string{"register", "--non-interactive", "--agree-tos", "-m", strings.TrimSpace(req.Email)}, flags...)
		out, err := exec.Command("certbot", args...).CombinedOutput()
		msg := strings.TrimSpace(string(out))
		if err != nil && !certbotAccountExists(msg) {
			if msg == "" {
				msg = err.Error()
			}
			return nil, fmt.Errorf("Let's Encrypt account: %s", tailOut(msg))
		}
		st = showLEAccount(flags)
		if !st.Registered {
			if certbotAccountExists(msg) {
				st.Registered = true
				st.Message = "Account already exists on this server"
			} else {
				return nil, fmt.Errorf("Let's Encrypt account was not created")
			}
		} else if st.Message == "" {
			st.Message = "Let's Encrypt account created"
		}
	} else if email := strings.TrimSpace(req.Email); email != "" && !strings.EqualFold(email, st.Email) {
		args := append([]string{"update_account", "--non-interactive", "-m", email}, flags...)
		if out, err := exec.Command("certbot", args...).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("Let's Encrypt account: %s", tailOut(strings.TrimSpace(string(out))))
		}
		st = showLEAccount(flags)
		if st.Message == "" {
			st.Message = "Let's Encrypt account email updated"
		}
	} else if st.Message == "" {
		st.Message = "Let's Encrypt account is already registered"
	}
	st.OK = st.Registered
	if st.Email == "" {
		st.Email = strings.TrimSpace(req.Email)
	}
	return st, nil
}

func (m *Manager) LEAccountStatus(req rpc.LEAccountReq) (*rpc.LEAccountResp, error) {
	if _, err := exec.LookPath("certbot"); err != nil {
		return &rpc.LEAccountResp{OK: true, CertbotInstalled: false, Message: "certbot is not installed"}, nil
	}
	flags, err := certbotACMEFlags(req.Server, req.Directory, req.NoVerify, req.EABKID, req.EABHMAC)
	if err != nil {
		return nil, err
	}
	st := showLEAccount(flags)
	st.OK = true
	return st, nil
}

func showLEAccount(flags []string) *rpc.LEAccountResp {
	args := append([]string{"show_account", "--non-interactive"}, flags...)
	out, err := exec.Command("certbot", args...).CombinedOutput()
	msg := strings.TrimSpace(string(out))
	uri, email, ok := parseCertbotAccount(msg)
	st := &rpc.LEAccountResp{CertbotInstalled: true, Registered: ok, URI: uri, Email: email}
	if err != nil && !ok {
		st.Message = tailOut(msg)
	}
	return st
}

func certbotACMEFlags(server, directory string, noVerify bool, eabKid, eabHmac string) ([]string, error) {
	dirURL, custom, err := acmeDirectory(rpc.SiteSSLReq{Server: server, Directory: directory})
	if err != nil {
		return nil, err
	}
	var args []string
	if dirURL != "" {
		args = append(args, "--server", dirURL)
	}
	if noVerify && custom {
		args = append(args, "--no-verify-ssl")
	}
	if kid := strings.TrimSpace(eabKid); kid != "" {
		hmac := strings.TrimSpace(eabHmac)
		if hmac == "" {
			return nil, fmt.Errorf("EAB HMAC key is required when EAB Key ID is set")
		}
		args = append(args, "--eab-kid", kid, "--eab-hmac-key", hmac)
	}
	return args, nil
}

func acmeDirectory(req rpc.SiteSSLReq) (dir string, custom bool, err error) {
	switch strings.ToLower(strings.TrimSpace(req.Server)) {
	case "", "production", "prod", "letsencrypt":
		return "", false, nil
	case "staging":
		return leStaging, false, nil
	case "custom":
		u := strings.TrimSpace(req.Directory)
		if u == "" {
			return "", true, fmt.Errorf("custom ACME directory URL is required")
		}
		parsed, perr := url.Parse(u)
		if perr != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
			return "", true, fmt.Errorf("invalid ACME directory URL")
		}
		return u, true, nil
	default:
		return "", false, fmt.Errorf("invalid ACME server")
	}
}

func publicACMEName(name string) bool {
	name = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
	if name == "" || strings.Contains(name, "*") {
		return false
	}
	parts := strings.Split(name, ".")
	if len(parts) < 2 {
		return false
	}
	tld := parts[len(parts)-1]
	switch tld {
	case "test", "localhost", "invalid", "example", "local", "onion", "internal", "lan", "home", "corp", "private", "localdomain":
		return false
	}
	return len(tld) >= 2
}

func tailOut(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 800 {
		return s
	}
	return s[len(s)-800:]
}

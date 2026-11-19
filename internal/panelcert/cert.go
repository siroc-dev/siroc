package panelcert

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/http01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/providers/http/webroot"
	"github.com/go-acme/lego/v4/registration"

	"github.com/siroc-dev/siroc/internal/setup"
)

const (
	Email       = "ssl@siroc.dev"
	Profile     = "shortlived"
	RenewBefore = 48 * time.Hour
	Webroot     = "/var/www/letsencrypt"
)

// Paths are the panel data directory and the certificate files it serves.
type Paths struct {
	DataDir  string
	CertFile string
	KeyFile  string
}

// Status describes the certificate currently on disk.
type Status struct {
	IP          string    `json:"ip"`
	Email       string    `json:"email"`
	Issuer      string    `json:"issuer"`
	NotAfter    time.Time `json:"notAfter"`
	IPs         []string  `json:"ips"`
	CoversIP    bool      `json:"coversIp"`
	LetsEncrypt bool      `json:"letsEncrypt"`
	Message     string    `json:"message,omitempty"`
}

type acmeUser struct {
	email string
	key   crypto.PrivateKey
	reg   *registration.Resource
}

func (u *acmeUser) GetEmail() string                        { return u.email }
func (u *acmeUser) GetRegistration() *registration.Resource { return u.reg }
func (u *acmeUser) GetPrivateKey() crypto.PrivateKey        { return u.key }

type savedAccount struct {
	KeyPEM       []byte                 `json:"keyPem"`
	Registration *registration.Resource `json:"registration"`
}

// Describe reads the panel certificate and the current public IPv4.
func Describe(certFile string) Status {
	st, _ := Read(certFile)
	st.Email = Email
	ip, err := setup.LookupPublicIPv4(nil, setup.PublicIPEndpoint)
	if err == nil && ip != "" {
		st.IP = ip
		st.CoversIP = hasIP(st.IPs, ip)
	}
	if st.Message == "" && st.IP == "" {
		st.Message = setup.PublicIPUnavailable
	}
	return st
}

// Ensure requests a short-lived Let's Encrypt certificate for the public IPv4
// when the file on disk does not cover that address or expires within two days.
// A certificate that is still valid is left in place.
func Ensure(ip string, paths Paths) (Status, error) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		found, err := setup.LookupPublicIPv4(nil, setup.PublicIPEndpoint)
		if err != nil || found == "" {
			return Status{Email: Email, Message: setup.PublicIPUnavailable}, fmt.Errorf("%s", setup.PublicIPUnavailable)
		}
		ip = found
	}
	if net.ParseIP(ip) == nil || net.ParseIP(ip).To4() == nil {
		return Status{Email: Email, IP: ip}, fmt.Errorf("public address %s is not an IPv4 address", ip)
	}
	current, _ := Read(paths.CertFile)
	current.Email = Email
	current.IP = ip
	current.CoversIP = hasIP(current.IPs, ip)
	if !NeedsRenew(current, ip, time.Now()) {
		current.Message = "Certificate is still valid"
		return current, nil
	}
	if err := obtain(ip, paths); err != nil {
		return current, err
	}
	st, err := Read(paths.CertFile)
	if err != nil {
		return Status{Email: Email, IP: ip}, err
	}
	st.Email = Email
	st.IP = ip
	st.CoversIP = hasIP(st.IPs, ip)
	st.Message = "Certificate issued. Open the panel in a new tab so the browser uses it."
	return st, nil
}

// NeedsRenew reports whether the certificate should be replaced for ip.
func NeedsRenew(st Status, ip string, now time.Time) bool {
	if !hasIP(st.IPs, ip) {
		return true
	}
	if st.NotAfter.IsZero() || !st.NotAfter.After(now.Add(RenewBefore)) {
		return true
	}
	return false
}

// Read parses the leaf certificate. A missing file returns an empty status.
func Read(certFile string) (Status, error) {
	b, err := os.ReadFile(certFile)
	if err != nil {
		if os.IsNotExist(err) {
			return Status{Email: Email}, nil
		}
		return Status{Email: Email}, err
	}
	rest := b
	var leaf *x509.Certificate
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return Status{Email: Email}, err
		}
		leaf = cert
		break
	}
	if leaf == nil {
		return Status{Email: Email}, fmt.Errorf("no certificate in %s", certFile)
	}
	ips := make([]string, 0, len(leaf.IPAddresses))
	for _, ip := range leaf.IPAddresses {
		ips = append(ips, ip.String())
	}
	issuer := leaf.Issuer.String()
	return Status{
		Email:       Email,
		Issuer:      issuer,
		NotAfter:    leaf.NotAfter,
		IPs:         ips,
		LetsEncrypt: strings.Contains(strings.ToLower(issuer), "let's encrypt"),
	}, nil
}

func obtain(ip string, paths Paths) error {
	if paths.DataDir == "" || paths.CertFile == "" || paths.KeyFile == "" {
		return fmt.Errorf("certificate path is not configured")
	}
	user, err := loadUser(paths.DataDir)
	if err != nil {
		return err
	}
	config := lego.NewConfig(user)
	config.CADirURL = lego.LEDirectoryProduction
	config.Certificate.KeyType = certcrypto.EC256
	config.Certificate.DisableCommonName = true
	config.Certificate.Timeout = 5 * time.Minute
	client, err := lego.NewClient(config)
	if err != nil {
		return err
	}
	if err := setChallenge(client); err != nil {
		return err
	}
	if user.reg == nil {
		reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
		if err != nil {
			reg, err = client.Registration.QueryRegistration()
			if err != nil {
				return fmt.Errorf("register Let's Encrypt account: %w", err)
			}
		}
		user.reg = reg
		if err := saveUser(paths.DataDir, user); err != nil {
			return err
		}
	}
	res, err := client.Certificate.Obtain(certificate.ObtainRequest{
		Domains: []string{ip},
		Bundle:  true,
		Profile: Profile,
	})
	if err != nil {
		return fmt.Errorf("Let's Encrypt: %w", err)
	}
	if err := writeFile(paths.CertFile, res.Certificate, 0644); err != nil {
		return err
	}
	if err := writeFile(paths.KeyFile, res.PrivateKey, 0640); err != nil {
		return err
	}
	matchDataOwner(paths.DataDir, paths.CertFile, paths.KeyFile)
	return nil
}

func setChallenge(client *lego.Client) error {
	ln, err := net.Listen("tcp", ":80")
	if err == nil {
		_ = ln.Close()
		return client.Challenge.SetHTTP01Provider(http01.NewProviderServer("", "80"))
	}
	dir := filepath.Join(Webroot, ".well-known", "acme-challenge")
	if mkErr := os.MkdirAll(dir, 0755); mkErr != nil {
		return fmt.Errorf("port 80 is in use and %s is not writable: %w", Webroot, mkErr)
	}
	provider, err := webroot.NewHTTPProvider(Webroot)
	if err != nil {
		return err
	}
	return client.Challenge.SetHTTP01Provider(provider)
}

func accountPath(dataDir string) string {
	return filepath.Join(dataDir, "acme", "account.json")
}

func loadUser(dataDir string) (*acmeUser, error) {
	b, err := os.ReadFile(accountPath(dataDir))
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}
		return &acmeUser{email: Email, key: key}, nil
	}
	var saved savedAccount
	if err := json.Unmarshal(b, &saved); err != nil {
		return nil, err
	}
	key, err := certcrypto.ParsePEMPrivateKey(saved.KeyPEM)
	if err != nil {
		return nil, err
	}
	return &acmeUser{email: Email, key: key, reg: saved.Registration}, nil
}

func saveUser(dataDir string, user *acmeUser) error {
	dir := filepath.Join(dataDir, "acme")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	body, err := json.Marshal(savedAccount{
		KeyPEM:       certcrypto.PEMEncode(user.key),
		Registration: user.reg,
	})
	if err != nil {
		return err
	}
	return writeFile(accountPath(dataDir), body, 0600)
}

func writeFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func hasIP(ips []string, ip string) bool {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return false
	}
	for _, item := range ips {
		if net.ParseIP(item).Equal(parsed) {
			return true
		}
	}
	return false
}

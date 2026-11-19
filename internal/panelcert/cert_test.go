package panelcert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNeedsRenew(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	fresh := Status{IPs: []string{"203.0.113.5"}, NotAfter: now.Add(5 * 24 * time.Hour)}
	if NeedsRenew(fresh, "203.0.113.5", now) {
		t.Fatal("fresh certificate should be kept")
	}
	if !NeedsRenew(fresh, "198.51.100.8", now) {
		t.Fatal("certificate for a different address should be replaced")
	}
	soon := fresh
	soon.NotAfter = now.Add(24 * time.Hour)
	if !NeedsRenew(soon, "203.0.113.5", now) {
		t.Fatal("certificate inside the renewal window should be replaced")
	}
	if !NeedsRenew(Status{}, "203.0.113.5", now) {
		t.Fatal("missing certificate should be requested")
	}
}

func TestReadCertificateAddresses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tls.crt")
	if err := os.WriteFile(path, testCert(t, net.ParseIP("203.0.113.5"), time.Now().Add(5*24*time.Hour)), 0644); err != nil {
		t.Fatal(err)
	}
	st, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if !hasIP(st.IPs, "203.0.113.5") {
		t.Fatalf("ips %v", st.IPs)
	}
	if st.LetsEncrypt {
		t.Fatal("test certificate must not look like Let's Encrypt")
	}
	if NeedsRenew(st, "203.0.113.5", time.Now()) {
		t.Fatal("certificate valid for five days should not renew")
	}
}

func TestEnsureKeepsFreshCertificate(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "tls.crt")
	keyFile := filepath.Join(dir, "tls.key")
	if err := os.WriteFile(certFile, testCert(t, net.ParseIP("203.0.113.5"), time.Now().Add(5*24*time.Hour)), 0644); err != nil {
		t.Fatal(err)
	}
	st, err := Ensure("203.0.113.5", Paths{DataDir: dir, CertFile: certFile, KeyFile: keyFile})
	if err != nil {
		t.Fatal(err)
	}
	if st.Message != "Certificate is still valid" {
		t.Fatalf("message %q", st.Message)
	}
	if !st.CoversIP {
		t.Fatal("status should cover the public address")
	}
}

func TestRenewUnits(t *testing.T) {
	service := RenewService("/siroc/data", "/siroc/bin/siroc-panel")
	if !strings.Contains(service, "Environment=SIROC_DATA_DIR=/siroc/data") || !strings.Contains(service, "ExecStart=/siroc/bin/siroc-panel issue-cert") {
		t.Fatal(service)
	}
	if !strings.Contains(RenewTimer(), "OnCalendar=daily") {
		t.Fatal(RenewTimer())
	}
}

func testCert(t *testing.T, ip net.IP, until time.Time) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "siroc"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     until,
		IPAddresses:  []net.IP{ip},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

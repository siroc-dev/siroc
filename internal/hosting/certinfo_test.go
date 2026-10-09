package hosting

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func testCert(t *testing.T, cn string, dns []string) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(7),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:     dns,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestParseCertPEM(t *testing.T) {
	raw := testCert(t, "site.test", []string{"site.test", "www.site.test"})
	info, err := ParseCertPEM(raw)
	if err != nil {
		t.Fatal(err)
	}
	if info.Subject != "site.test" || info.Issuer != "site.test" {
		t.Fatalf("subject %q issuer %q", info.Subject, info.Issuer)
	}
	if len(info.DNSNames) != 2 || info.NotAfter == "" || info.Serial == "" {
		t.Fatalf("%#v", info)
	}
}

func TestCertCovers(t *testing.T) {
	raw := testCert(t, "*.site.test", []string{"*.site.test"})
	if !CertCovers(raw, "www.site.test") {
		t.Fatal("wildcard should cover www")
	}
	if CertCovers(raw, "site.test") {
		t.Fatal("wildcard should not cover the apex")
	}
	if CertCovers(raw, "a.b.site.test") {
		t.Fatal("wildcard should not cover a nested name")
	}
}

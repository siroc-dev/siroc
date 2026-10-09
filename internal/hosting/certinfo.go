package hosting

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

const maxCertPEM = 256 << 10

// ParseCertPEM reads the first certificate in a PEM bundle.
func ParseCertPEM(raw []byte) (rpc.CertInfo, error) {
	if len(raw) == 0 || len(raw) > maxCertPEM {
		return rpc.CertInfo{}, fmt.Errorf("certificate is empty or too large")
	}
	rest := raw
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
			return rpc.CertInfo{}, fmt.Errorf("certificate: %w", err)
		}
		if leaf == nil {
			leaf = cert
		}
	}
	if leaf == nil {
		return rpc.CertInfo{}, fmt.Errorf("no certificate in PEM")
	}
	info := rpc.CertInfo{
		OK:        true,
		Subject:   leaf.Subject.CommonName,
		Issuer:    leaf.Issuer.CommonName,
		NotBefore: leaf.NotBefore.UTC().Format(time.RFC3339),
		NotAfter:  leaf.NotAfter.UTC().Format(time.RFC3339),
		DNSNames:  uniqueNames(leaf),
		Serial:    leaf.SerialNumber.Text(16),
	}
	if info.Subject == "" {
		info.Subject = leaf.Subject.String()
	}
	if info.Issuer == "" {
		info.Issuer = leaf.Issuer.String()
	}
	return info, nil
}

func uniqueNames(cert *x509.Certificate) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(n string) {
		n = strings.TrimSpace(n)
		if n == "" {
			return
		}
		key := strings.ToLower(n)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, n)
	}
	add(cert.Subject.CommonName)
	for _, n := range cert.DNSNames {
		add(n)
	}
	return out
}

// CertCovers reports whether the leaf certificate is valid for domain.
func CertCovers(raw []byte, domain string) bool {
	rest := raw
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
			return false
		}
		leaf = cert
		break
	}
	if leaf == nil {
		return false
	}
	return nameCovers(leaf, domain)
}

func nameCovers(cert *x509.Certificate, domain string) bool {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return false
	}
	names := cert.DNSNames
	if len(names) == 0 && cert.Subject.CommonName != "" {
		names = []string{cert.Subject.CommonName}
	}
	for _, n := range names {
		n = strings.ToLower(strings.TrimSpace(n))
		if n == domain {
			return true
		}
		if strings.HasPrefix(n, "*.") {
			parent := strings.TrimPrefix(n, "*")
			rest := strings.TrimSuffix(domain, parent)
			if rest != domain && rest != "" && !strings.Contains(rest, ".") {
				return true
			}
		}
	}
	return false
}

// MatchCertKey checks that the private key belongs to the certificate.
func MatchCertKey(certPEM, keyPEM []byte) error {
	if len(certPEM) == 0 || len(certPEM) > maxCertPEM || len(keyPEM) == 0 || len(keyPEM) > maxCertPEM {
		return fmt.Errorf("certificate or key is empty or too large")
	}
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return fmt.Errorf("certificate and key do not match")
	}
	return nil
}

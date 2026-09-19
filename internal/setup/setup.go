package setup

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

const (
	TokenFile = "setup.token"
	URLFile   = "setup.url"
	tokenBytes = 32
)

func TokenPath(dataDir string) string {
	return filepath.Join(dataDir, TokenFile)
}

func URLPath(dataDir string) string {
	return filepath.Join(dataDir, URLFile)
}

func Load(dataDir string) (string, error) {
	b, err := os.ReadFile(TokenPath(dataDir))
	if err != nil {
		return "", err
	}
	tok := strings.TrimSpace(string(b))
	if tok == "" {
		return "", fmt.Errorf("empty setup token")
	}
	return tok, nil
}

func Ensure(dataDir string) (string, error) {
	if tok, err := Load(dataDir); err == nil {
		return tok, nil
	}
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	tok := hex.EncodeToString(raw)
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return "", err
	}
	if err := os.WriteFile(TokenPath(dataDir), []byte(tok+"\n"), 0600); err != nil {
		return "", err
	}
	return tok, nil
}

func Valid(dataDir, token string) bool {
	want, err := Load(dataDir)
	if err != nil || token == "" {
		return false
	}
	if len(token) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(want)) == 1
}

func Clear(dataDir string) {
	_ = os.Remove(TokenPath(dataDir))
	_ = os.Remove(URLPath(dataDir))
}

func AdvertiseHost() string {
	if h := os.Getenv("SIROC_PUBLIC_HOST"); h != "" {
		return h
	}
	addrs, err := net.InterfaceAddrs()
	if err == nil {
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.IsLoopback() {
				continue
			}
			if v4 := ipn.IP.To4(); v4 != nil {
				return v4.String()
			}
		}
	}
	return "127.0.0.1"
}

func PublicURL(listenAddr string, tlsOn bool, token string) string {
	scheme := "https"
	if !tlsOn {
		scheme = "http"
	}
	host := AdvertiseHost()
	port := listenPort(listenAddr)
	if (tlsOn && port == "443") || (!tlsOn && port == "80") {
		return fmt.Sprintf("%s://%s/setup?token=%s", scheme, host, token)
	}
	return fmt.Sprintf("%s://%s:%s/setup?token=%s", scheme, host, port, token)
}

func WriteURL(dataDir, url string) error {
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return err
	}
	return os.WriteFile(URLPath(dataDir), []byte(url+"\n"), 0600)
}

func listenPort(addr string) string {
	if addr == "" {
		return "8443"
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "8443"
	}
	if port == "" {
		return "8443"
	}
	return port
}

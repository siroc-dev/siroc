package setup

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	PublicIPEndpoint    = "https://api.ipify.org"
	PublicIPUnavailable = "could not detect a public IP"
)

// PrivateHost is the first non-loopback IPv4 address on this machine.
func PrivateHost() string {
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

// PanelURL builds a panel origin, or a token setup URL when token is set.
func PanelURL(listenAddr string, tlsOn bool, host, token string) string {
	scheme := "https"
	if !tlsOn {
		scheme = "http"
	}
	host = strings.TrimSpace(host)
	if host == "" {
		host = "127.0.0.1"
	}
	port := listenPort(listenAddr)
	var base string
	if (tlsOn && port == "443") || (!tlsOn && port == "80") {
		base = fmt.Sprintf("%s://%s", scheme, host)
	} else {
		base = fmt.Sprintf("%s://%s:%s", scheme, host, port)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return base
	}
	return base + "/setup?token=" + token
}

// FormatLinks renders the private and public panel links.
// One line is returned when both hosts are the same address.
func FormatLinks(listenAddr string, tlsOn bool, token, privateHost, publicHost string) string {
	privateHost = strings.TrimSpace(privateHost)
	if privateHost == "" {
		privateHost = "127.0.0.1"
	}
	publicHost = strings.TrimSpace(publicHost)
	priv := PanelURL(listenAddr, tlsOn, privateHost, token)
	if publicHost == "" {
		return fmt.Sprintf("Private: %s\nPublic:  %s", priv, PublicIPUnavailable)
	}
	if publicHost == privateHost {
		return priv
	}
	pub := PanelURL(listenAddr, tlsOn, publicHost, token)
	return fmt.Sprintf("Private: %s\nPublic:  %s", priv, pub)
}

// LookupPublicIPv4 reads an IPv4 address from an echo service.
func LookupPublicIPv4(client *http.Client, endpoint string) (string, error) {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	if strings.TrimSpace(endpoint) == "" {
		endpoint = PublicIPEndpoint
	}
	resp, err := client.Get(endpoint)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("public IP lookup returned %s", resp.Status)
	}
	raw := strings.TrimSpace(string(body))
	ip := net.ParseIP(raw)
	if ip == nil || ip.To4() == nil || ip.IsLoopback() || ip.IsUnspecified() {
		return "", fmt.Errorf("public IP lookup returned %q", raw)
	}
	return ip.To4().String(), nil
}

// SaveLinks checks the public address before writing both setup links.
// A failed lookup still writes the private link and does not fail setup.
func SaveLinks(dataDir, listenAddr string, tlsOn bool, token string) (string, error) {
	public := strings.TrimSpace(os.Getenv("SIROC_PUBLIC_HOST"))
	if public == "" {
		ip, err := LookupPublicIPv4(nil, PublicIPEndpoint)
		if err == nil {
			public = ip
		}
	}
	text := FormatLinks(listenAddr, tlsOn, token, PrivateHost(), public)
	if err := WriteURL(dataDir, text); err != nil {
		return text, err
	}
	return text, nil
}

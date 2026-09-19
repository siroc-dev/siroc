package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	ListenAddr   string
	TLSCert      string
	TLSKey       string
	DisableTLS   bool
	DataDir      string
	DBPath       string
	SocketPath   string
	InstallRoot  string
	HomeRoot     string
	NginxSites   string
	ApacheSites  string
	PHPFPMPool   string
	Bootstrap    bool
}

func Load() Config {
	dataDir := env("SIROC_DATA_DIR", "/var/lib/siroc")
	c := Config{
		ListenAddr:  env("SIROC_LISTEN", ":8443"),
		TLSCert:     env("SIROC_TLS_CERT", filepath.Join(dataDir, "tls.crt")),
		TLSKey:      env("SIROC_TLS_KEY", filepath.Join(dataDir, "tls.key")),
		DisableTLS:  env("SIROC_TLS", "1") == "0",
		DataDir:     dataDir,
		DBPath:      env("SIROC_DB", filepath.Join(dataDir, "panel.db")),
		SocketPath:  env("SIROC_SOCKET", "/var/run/siroc/agent.sock"),
		InstallRoot: env("SIROC_ROOT", "/opt/siroc"),
		HomeRoot:    env("SIROC_HOME_ROOT", "/home"),
		NginxSites:  env("SIROC_NGINX_SITES", "/etc/nginx/sites-enabled"),
		ApacheSites: env("SIROC_APACHE_SITES", "/etc/apache2/sites-enabled"),
		PHPFPMPool:  env("SIROC_PHP_POOL", "/etc/php"),
		Bootstrap:   env("SIROC_BOOTSTRAP", "") == "1",
	}
	return c
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

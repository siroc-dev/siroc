package software

var descriptions = map[string]string{
	"nginx":      "Public HTTP frontend. Terminates TLS and proxies each site to Apache or an app port.",
	"apache":     "PHP backend behind Nginx. Serves document roots and runs ModSecurity.",
	"php":        "PHP-FPM pools per website. Install extra versions for older or newer apps.",
	"mysql":      "MySQL server for site databases. Cannot be installed next to MariaDB.",
	"mariadb":    "MariaDB server for site databases. Default first-run database engine.",
	"redis":      "In-memory cache and queue broker. Accounts can get an isolated Redis user.",
	"python":     "Python runtimes for app sites. Sets the default python3 CLI.",
	"nodejs":     "Node.js runtimes for app sites. Sets the default node CLI.",
	"golang":     "Go toolchain for compiling and running Go websites.",
	"rust":       "Rustc and Cargo for Rust application sites.",
	"docker":     "Docker Engine and Compose for containerized sites.",
	"certbot":    "Let's Encrypt client. Issues and renews public HTTPS certificates.",
	"ufw":        "Host firewall. Opens only the ports the panel needs by default.",
	"waf":        "ModSecurity with OWASP CRS on Apache. Hosting users can disable it per account or site.",
	"clamav":     "Antivirus scanner for files on the server.",
	"openssh":    "SSH server. Hosting accounts can be granted or denied shell access.",
	"vsftpd":     "FTP server. Hosting accounts can be granted or denied FTP.",
	"goaccess":   "Access-log analytics. Builds per-site HTML stats from Nginx logs.",
	"supervisor": "Process manager for Laravel queue workers and scheduled jobs.",
	"composer":   "PHP dependency manager for Laravel and other Composer apps.",
	"wp-cli":     "WordPress command-line tool for installs, plugins, and users.",
	"nikto":      "Web vulnerability scanner. Admin-only scan reports.",
	"zap":        "OWASP ZAP scanner for authenticated web tests.",
	"openvas":    "OpenVAS / Greenbone vulnerability management.",
	"memcached":  "Memory object cache for PHP and application backends.",
	"ffmpeg":     "Media converter for video and audio processing.",
	"fail2ban":   "Bans IPs after repeated SSH and service failures.",
	"phpmyadmin": "Browser UI for MySQL and MariaDB, served from the panel.",
	"quota":      "Disk quotas so each hosting account has a storage limit.",
}

func Description(name string) string {
	return descriptions[name]
}

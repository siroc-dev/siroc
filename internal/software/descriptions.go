package software

var descriptions = map[string]string{
	"nginx":      "Public HTTP frontend. Terminates TLS and proxies each site to Apache or an app port.",
	"apache":     "PHP backend behind Nginx. Serves document roots and runs ModSecurity.",
	"php":        "PHP-FPM pools per website. Extra versions come from Ondrej Sury (packages.sury.org) when that release has a repo.",
	"mysql":      "MySQL compiled from official source into /opt/siroc/db/mysql. Cannot sit next to MariaDB.",
	"mariadb":    "MariaDB compiled from official source into /opt/siroc/db/mariadb. Default first-run database.",
	"redis":      "In-memory cache and queue broker. Accounts can get an isolated Redis user.",
	"python":     "Python runtimes for app sites. Sets the default python3 CLI.",
	"nodejs":     "Node.js runtimes for app sites. Sets the default node CLI.",
	"golang":     "Go toolchain for compiling and running Go websites.",
	"rust":       "Rustc and Cargo for Rust application sites.",
	"docker":     "Docker Engine and Compose for containerized sites.",
	"certbot":    "Let's Encrypt client. Issues and renews public HTTPS certificates.",
	"ufw":        "Host firewall. Opens only the ports the panel needs by default.",
	"waf":        "ModSecurity with OWASP CRS on Apache. Can scan HTTP and file-manager uploads with ClamAV.",
	"clamav":     "Antivirus scanner for files on the server. Used by WAF upload virus scan.",
	"openssh":    "SSH server. Hosting accounts can be granted or denied shell access.",
	"vsftpd":     "FTP server. Hosting accounts can be granted or denied FTP.",
	"goaccess":   "Access-log analytics with time distribution, geo location, ASN mapping, and AI crawlers.",
	"supervisor": "Process manager for Laravel queue workers and scheduled jobs.",
	"git":        "Git client for site deploy. First-run stack installs it so force-pull webhooks work.",
	"composer":   "PHP dependency manager for Laravel and other Composer apps.",
	"wp-cli":     "WordPress command-line tool for installs, plugins, and users.",
	"nikto":      "Web vulnerability scanner. Admin-only scan reports.",
	"zap":        "OWASP ZAP scanner for authenticated web tests.",
	"openvas":    "OpenVAS / Greenbone vulnerability management.",
	"memcached":  "Memory object cache for PHP and application backends.",
	"ffmpeg":     "Media converter for video and audio processing.",
	"fail2ban":   "Bans IPs after repeated SSH, FTP, MySQL/MariaDB, Redis, Nginx, Apache, and Siroc panel login failures.",
	"phpmyadmin": "Browser UI for MySQL and MariaDB, served from the panel.",
	"quota":      "Disk quotas so each hosting account has a storage limit.",
}

func Description(name string) string {
	return descriptions[name]
}

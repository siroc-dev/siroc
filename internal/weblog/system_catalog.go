package weblog

import "strings"

type systemLogSpec struct {
	ID    string
	Group string
	Title string
	Path  string
	Kind  string
	Unit  string
}

func systemLogCatalog() []systemLogSpec {
	return []systemLogSpec{
		{ID: "panel-auth", Group: "panel", Title: "Panel login", Path: "/var/log/siroc/auth.log", Kind: "file"},
		{ID: "panel-journal", Group: "panel", Title: "Panel service", Kind: "journal", Unit: "siroc-panel"},
		{ID: "agent-journal", Group: "panel", Title: "Agent service", Kind: "journal", Unit: "siroc-agent"},

		{ID: "nginx-error", Group: "server", Title: "Nginx error", Path: "/var/log/nginx/error.log", Kind: "file"},
		{ID: "nginx-access", Group: "server", Title: "Nginx access", Path: "/var/log/nginx/access.log", Kind: "file"},
		{ID: "apache-error", Group: "server", Title: "Apache error", Path: "/var/log/apache2/error.log", Kind: "file"},
		{ID: "apache-access", Group: "server", Title: "Apache access", Path: "/var/log/apache2/access.log", Kind: "file"},
		{ID: "waf-audit", Group: "server", Title: "ModSecurity audit", Path: "/var/log/apache2/modsec_audit.log", Kind: "file"},
		{ID: "mysql-error", Group: "server", Title: "MySQL error", Path: "/var/log/mysql/error.log", Kind: "file"},
		{ID: "mysqld-error", Group: "server", Title: "mysqld error", Path: "/var/log/mysqld.log", Kind: "file"},
		{ID: "mariadb-error", Group: "server", Title: "MariaDB error", Path: "/var/log/mysql/error.log", Kind: "file"},
		{ID: "redis", Group: "server", Title: "Redis", Path: "/var/log/redis/redis-server.log", Kind: "file"},
		{ID: "fail2ban", Group: "server", Title: "Fail2ban", Path: "/var/log/fail2ban.log", Kind: "file"},
		{ID: "ufw", Group: "server", Title: "UFW", Path: "/var/log/ufw.log", Kind: "file"},
		{ID: "clamav", Group: "server", Title: "ClamAV", Path: "/var/log/clamav/clamav.log", Kind: "file"},

		{ID: "ssh-auth", Group: "ssh", Title: "auth.log", Path: "/var/log/auth.log", Kind: "file"},
		{ID: "ssh-secure", Group: "ssh", Title: "secure", Path: "/var/log/secure", Kind: "file"},
		{ID: "ssh-last", Group: "ssh", Title: "Recent logins", Kind: "command"},

		{ID: "cron-log", Group: "cron", Title: "cron", Path: "/var/log/cron", Kind: "file"},
		{ID: "cron-log-alt", Group: "cron", Title: "cron.log", Path: "/var/log/cron.log", Kind: "file"},
		{ID: "cron-journal", Group: "cron", Title: "Cron service", Kind: "journal", Unit: "cron"},
		{ID: "syslog", Group: "cron", Title: "syslog", Path: "/var/log/syslog", Kind: "file"},
		{ID: "messages", Group: "cron", Title: "messages", Path: "/var/log/messages", Kind: "file"},
	}
}

func lookupSystemLog(id string) (systemLogSpec, bool) {
	id = strings.TrimSpace(id)
	for _, s := range systemLogCatalog() {
		if s.ID == id {
			return s, true
		}
	}
	return systemLogSpec{}, false
}

func phpFpmLogID(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if !strings.HasPrefix(name, "php") || !strings.HasSuffix(name, "-fpm.log") {
		return "", false
	}
	ver := strings.TrimSuffix(strings.TrimPrefix(name, "php"), "-fpm.log")
	if ver == "" || strings.ContainsAny(ver, "/\\") {
		return "", false
	}
	return "php-fpm-" + ver, true
}

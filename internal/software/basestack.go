package software

import (
	"fmt"
	"strings"
)

// Versions offered by the panel. The first-login prompt installs a chosen subset.
const (
	BasePHPVersion     = "8.4"
	BaseMariaDBVersion = "11.4"
	BaseMySQLVersion   = "8.4"
	BaseRedisVersion   = "7.2"
	BaseNginxVersion   = "1.30"
	BaseApacheVersion  = "2.4"
)

type StackPkg struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Version string `json:"version"`
}

// RecommendedInstall is the first-login set: Nginx, Apache, exactly one database,
// Redis, Let's Encrypt, and FTP. db is "mysql" or "mariadb".
func RecommendedInstall(db string) ([]StackPkg, error) {
	switch strings.TrimSpace(strings.ToLower(db)) {
	case "", "mariadb":
		db = "mariadb"
	case "mysql":
	default:
		return nil, fmt.Errorf("choose MySQL or MariaDB")
	}
	database := StackPkg{Name: "mariadb", Title: "MariaDB", Version: BaseMariaDBVersion}
	if db == "mysql" {
		database = StackPkg{Name: "mysql", Title: "MySQL", Version: BaseMySQLVersion}
	}
	return []StackPkg{
		{Name: "nginx", Title: "Nginx", Version: BaseNginxVersion},
		{Name: "apache", Title: "Apache", Version: BaseApacheVersion},
		database,
		{Name: "redis", Title: "Redis", Version: BaseRedisVersion},
		{Name: "certbot", Title: "Let's Encrypt"},
		{Name: "vsftpd", Title: "FTP"},
	}, nil
}

func BaseStack() []StackPkg {
	return []StackPkg{
		{Name: "nginx", Title: "Nginx", Version: ""},
		{Name: "apache", Title: "Apache", Version: ""},
		{Name: "php", Title: "PHP-FPM", Version: BasePHPVersion},
		{Name: "mariadb", Title: "MariaDB", Version: BaseMariaDBVersion},
		{Name: "git", Title: "Git", Version: ""},
		{Name: "ufw", Title: "UFW firewall", Version: ""},
		{Name: "fail2ban", Title: "Fail2ban", Version: ""},
	}
}

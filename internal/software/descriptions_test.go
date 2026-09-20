package software

import "testing"

func TestDescriptionCoversCatalog(t *testing.T) {
	names := []string{
		"nginx", "apache", "php", "mysql", "mariadb", "redis", "python", "nodejs",
		"golang", "rust", "docker", "certbot", "ufw", "waf", "clamav", "openssh",
		"vsftpd", "goaccess", "supervisor", "composer", "wp-cli", "nikto", "zap",
		"openvas", "memcached", "ffmpeg", "fail2ban", "phpmyadmin", "quota",
	}
	for _, name := range names {
		if Description(name) == "" {
			t.Fatalf("missing description for %s", name)
		}
	}
	if Description("unknown-pkg") != "" {
		t.Fatal("unknown package should have empty description")
	}
}

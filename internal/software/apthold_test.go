package software

import (
	"strings"
	"testing"
)

func TestExpandHoldKeepsPHPFamiliesApart(t *testing.T) {
	installed := []string{
		"php8.4-fpm", "php8.4-cli", "php8.4-common", "php8.4-json",
		"php7.4-fpm", "php7.4-cli", "php7.4-json", "php7.4-common",
		"php-pear", "php-cli",
	}
	got := expandHold([]string{"php7.4-fpm"}, installed)
	want := []string{"php7.4-cli", "php7.4-common", "php7.4-fpm", "php7.4-json"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v", got)
	}
}

func TestExpandHoldNginxAndMariaDB(t *testing.T) {
	installed := []string{"nginx", "nginx-common", "libnginx-mod-http-image-filter", "mariadb-server", "mariadb-client", "mysql-common", "mysql-server", "git"}
	got := expandHold([]string{"nginx", "mariadb-server"}, installed)
	want := []string{"libnginx-mod-http-image-filter", "mariadb-client", "mariadb-server", "mysql-common", "nginx", "nginx-common"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v", got)
	}
}

func TestExpandHoldPythonDoesNotCatchDistroPackages(t *testing.T) {
	installed := []string{"python3", "python3-apt", "python3.12", "python3.12-minimal", "python3.12-venv", "python3.11", "python3.11-venv"}
	got := expandHold([]string{"python3.12"}, installed)
	want := []string{"python3.12", "python3.12-minimal", "python3.12-venv"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v", got)
	}
}

func TestReleaseNamesLeavesOtherPHP(t *testing.T) {
	installed := []string{"php8.4-fpm", "php8.4-cli", "php7.4-fpm", "nginx"}
	recorded := []string{"php8.4-fpm", "php8.4-cli", "php7.4-fpm", "nginx"}
	got := releaseNames([]string{"php7.4-fpm", "php7.4-cli", "php7.4-json"}, recorded, installed)
	if strings.Join(got, ",") != "php7.4-fpm" {
		t.Fatalf("got %v", got)
	}
}

func TestShouldSeedSkipsSSHAndPython(t *testing.T) {
	if ShouldSeed("openssh-server") || ShouldSeed("python3.12") || ShouldSeed("build-essential") || ShouldSeed("ca-certificates") {
		t.Fatal("ssh, system python, and build tools must stay on apt")
	}
	for _, pkg := range []string{"nginx", "php8.4-fpm", "php7.4-json", "mariadb-server", "apache2-bin", "redis-server", "git", "fail2ban"} {
		if !ShouldSeed(pkg) {
			t.Fatal(pkg)
		}
	}
}

func TestDebInstalledStatusAcceptsHold(t *testing.T) {
	if !debInstalledStatus("hold ok installed") || !debInstalledStatus("install ok installed") {
		t.Fatal("held and normal packages are installed")
	}
	for _, status := range []string{"hold ok not-installed", "deinstall ok config-files", "install ok half-configured", "install ok unpacked"} {
		if debInstalledStatus(status) {
			t.Fatal(status)
		}
	}
}

func TestParseManagedIgnoresComment(t *testing.T) {
	text := "# Installed by Siroc. apt upgrade skips these. Update them from Software.\nnginx\n\nphp8.4-fpm\n"
	got := parseManaged(text)
	if strings.Join(got, ",") != "nginx,php8.4-fpm" {
		t.Fatalf("got %v", got)
	}
	if !strings.Contains(formatManaged(got), "apt upgrade skips these") {
		t.Fatal(formatManaged(got))
	}
}

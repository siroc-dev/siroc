package hosting

import "testing"

func TestPHPVerFromApacheConf(t *testing.T) {
	conf := "DocumentRoot /home/demo/domains/shop.test/public_html\nSetHandler \"proxy:unix:/run/php/php8.3-demo-shop_test.sock|fcgi://localhost\"\n"
	if got := phpVerFromApacheConf(conf); got != "8.3" {
		t.Fatalf("version %q", got)
	}
	if phpVerFromApacheConf("DocumentRoot /var/www") != "" {
		t.Fatal("expected no version")
	}
}

func TestWPLoaderNoise(t *testing.T) {
	msg := "Script error: the ionCube Loader for PHP needs to be installed."
	if !wpLoaderNoise(msg) {
		t.Fatal("expected ionCube output to be rejected")
	}
	if wpLoaderNoise("doujinmoon") || wpLoaderNoise("WordPress 7.1.4") {
		t.Fatal("real site fields should be kept")
	}
	fatal := "PHP Fatal error: No decryption key available - Unable to call protected function de8411"
	if !wpLoaderNoise(fatal) {
		t.Fatal("expected a protected-function failure to be rejected")
	}
	if cleanWPField(fatal) != "" || cleanWPField("  doujinmoon  ") != "doujinmoon" {
		t.Fatal("clean field")
	}
}

func TestWithWPSkipThemes(t *testing.T) {
	got := withWPSkipThemes([]string{"user", "list", "--role=administrator"})
	if len(got) != 4 || got[3] != "--skip-themes" {
		t.Fatalf("skip flag: %#v", got)
	}
	again := withWPSkipThemes([]string{"plugin", "list", "--skip-themes"})
	if len(again) != 3 {
		t.Fatalf("duplicate skip: %#v", again)
	}
}

func TestHostFromDocPath(t *testing.T) {
	if got := hostFromDocPath("/home/doujinmoon/domains/doujinmoon.com/public_html"); got != "doujinmoon.com" {
		t.Fatalf("host %q", got)
	}
	if hostFromDocPath("/var/www/html") != "" {
		t.Fatal("expected no host")
	}
}

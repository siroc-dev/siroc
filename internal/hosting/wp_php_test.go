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
}

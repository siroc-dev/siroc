package software

import "testing"

func TestIoncubeLoaderSO(t *testing.T) {
	if ioncubeLoaderSO("8.4") != "ioncube_loader_lin_8.4.so" {
		t.Fatal(ioncubeLoaderSO("8.4"))
	}
	if ioncubeLoaderSO("7.4") != "ioncube_loader_lin_7.4.so" {
		t.Fatal(ioncubeLoaderSO("7.4"))
	}
}

func TestIoncubeArchiveURL(t *testing.T) {
	url, err := ioncubeArchiveURL("x86_64")
	if err != nil || url != "https://downloads.ioncube.com/loader_downloads/ioncube_loaders_lin_x86-64.tar.gz" {
		t.Fatal(url, err)
	}
	if _, err := ioncubeArchiveURL("i686"); err == nil {
		t.Fatal("expected unsupported arch")
	}
}

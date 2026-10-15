package validate

import "testing"

func TestProxyURLHostname(t *testing.T) {
	got, err := ProxyURL("https://doujinfast.com")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://doujinfast.com/" {
		t.Fatalf("got %s", got)
	}
	got, err = ProxyURL("https://doujinfast.com/app")
	if err != nil || got != "https://doujinfast.com/app" {
		t.Fatalf("path got %s err %v", got, err)
	}
	got, err = ProxyURL("http://127.0.0.1:3000")
	if err != nil || got != "http://127.0.0.1:3000/" {
		t.Fatalf("ip got %s err %v", got, err)
	}
	if _, err := ProxyURL("doujinfast.com"); err == nil {
		t.Fatal("expected a scheme")
	}
}

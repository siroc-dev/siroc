package software

import (
	"strings"
	"testing"
)

func TestVSFTPDConfigAllowsNologin(t *testing.T) {
	conf := vsftpdConfig("203.0.113.10")
	for _, want := range []string{
		"check_shell=NO",
		"pam_service_name=siroc",
		"pasv_address=203.0.113.10",
		"allow_writeable_chroot=YES",
	} {
		if !strings.Contains(conf, want) {
			t.Fatalf("missing %q", want)
		}
	}
	pam := vsftpdPAM()
	if strings.Contains(pam, "pam_shells") {
		t.Fatal("pam stack still rejects nologin shells")
	}
	if !strings.Contains(pam, "@include common-auth") {
		t.Fatal("pam stack missing common-auth")
	}
}

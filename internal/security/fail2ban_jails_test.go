package security

import (
	"strings"
	"testing"
)

func TestRenderFail2banJails(t *testing.T) {
	got := RenderFail2banJails([]JailSpec{
		{Name: "siroc", Filter: "siroc", Logs: []string{"/var/log/siroc/auth.log"}, Port: "8443,80,443"},
		{Name: "vsftpd", Filter: "vsftpd", Logs: []string{"/var/log/vsftpd.log", "/var/log/auth.log"}, Port: "ftp"},
		{Name: "mysql", Filter: "mysqld-auth", Logs: []string{"/var/log/mysql/error.log"}, Port: "3306"},
		{Name: "redis", Filter: "siroc-redis", Logs: []string{"/var/log/redis/redis-server.log"}, Port: "6379"},
		{Name: "skip", Filter: "x"},
	})
	for _, want := range []string{
		"[siroc]",
		"filter = siroc",
		"port = 8443,80,443",
		"[vsftpd]",
		"logpath = /var/log/vsftpd.log",
		"          /var/log/auth.log",
		"[mysql]",
		"filter = mysqld-auth",
		"[redis]",
		"port = 6379",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q\n%s", want, got)
		}
	}
	if strings.Contains(got, "[skip]") {
		t.Fatal("empty log jail was rendered")
	}
}

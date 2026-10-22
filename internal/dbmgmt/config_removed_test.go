package dbmgmt

import (
	"strings"
	"testing"
)

func TestStripRemovedMariaDBOptions(t *testing.T) {
	in := "# managed by siroc\n# siroc-ram-gb=4\n[mysqld]\ninnodb_buffer_pool_size = 2G\ninnodb_buffer_pool_instances = 2\ninnodb-page-cleaners=1\nquery_cache_type = 0\n"
	got, changed := stripRemovedMariaDBOptions(in)
	if !changed {
		t.Fatal("expected a change")
	}
	if strings.Contains(got, "buffer_pool_instances") || strings.Contains(got, "page-cleaners") || strings.Contains(got, "page_cleaners") {
		t.Fatalf("removed option still present:\n%s", got)
	}
	for _, keep := range []string{"# managed by siroc", "innodb_buffer_pool_size = 2G", "query_cache_type = 0", "[mysqld]"} {
		if !strings.Contains(got, keep) {
			t.Fatalf("missing %q in\n%s", keep, got)
		}
	}
	if _, again := stripRemovedMariaDBOptions(got); again {
		t.Fatal("second pass should be a no-op")
	}
}

func TestMariaDBDefaultsMessages(t *testing.T) {
	dir := missingDefaultsDir("mariadbd: Can't read dir of '/etc/mysql/mariadb.conf.d/' (Errcode: 2 \"No such file or directory\")\nFatal error in defaults handling. Program aborted\n")
	if dir != "/etc/mysql/mariadb.conf.d" {
		t.Fatalf("dir %q", dir)
	}
	if missingDefaultsDir("Can't read dir of '/etc/mysql/../conf.d/'") != "" {
		t.Fatal("rejected a path with ..")
	}
	opt := unknownDefaultsOption("[ERROR] /usr/sbin/mariadbd: unknown variable 'innodb_buffer_pool_instances=1'\nFatal error in defaults handling. Program aborted\n")
	if opt != "innodb_buffer_pool_instances" {
		t.Fatalf("opt %q", opt)
	}
	body, changed := stripMySQLOption("innodb_buffer_pool_size = 2G\ninnodb-buffer-pool-instances = 1\n", opt)
	if !changed || strings.Contains(body, "instances") || !strings.Contains(body, "buffer_pool_size") {
		t.Fatalf("strip %v\n%s", changed, body)
	}
}

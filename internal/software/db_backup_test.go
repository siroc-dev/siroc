package software

import (
	"testing"
	"time"
)

func TestPickMySQLDataDirPrefersUbuntuPath(t *testing.T) {
	got := pickMySQLDataDir([]string{"/var/lib/mysql", "/var/lib/mariadb"})
	if got != "/var/lib/mariadb" {
		t.Fatal(got)
	}
	if pickMySQLDataDir(nil) != "" {
		t.Fatal("empty")
	}
}

func TestPreUpgradeDumpName(t *testing.T) {
	now := time.Date(2026, 10, 25, 1, 24, 0, 0, time.UTC)
	if got := preUpgradeDumpName("mariadb", now); got != "mariadb-20261025-012400.sql.gz" {
		t.Fatal(got)
	}
	if got := preUpgradeDumpName("mysql", now); got != "mysql-20261025-012400.sql.gz" {
		t.Fatal(got)
	}
}

func TestDumpsToRemoveKeepsNewestThree(t *testing.T) {
	names := []string{
		"mariadb-20261025-010000.sql.gz",
		"notes.txt",
		"mysql-20261025-030000.sql.gz",
		"mariadb-20261025-020000.sql.gz",
		"mariadb-20261025-040000.sql.gz",
	}
	got := dumpsToRemove(names, 3)
	if len(got) != 1 || got[0] != "mariadb-20261025-010000.sql.gz" {
		t.Fatalf("%v", got)
	}
	if dumpsToRemove(names[:3], 3) != nil {
		t.Fatal("fewer than keep should remove nothing")
	}
}

//go:build linux

package dbmgmt

import "testing"

func TestMariaDBTuneOmitsRemovedOptions(t *testing.T) {
	for name := range mysqlTune(4, true) {
		if removedMariaDBOption(name) {
			t.Fatalf("MariaDB tune still sets %s", name)
		}
	}
	if mysqlTune(4, false)["innodb_buffer_pool_instances"] == "" {
		t.Fatal("MySQL tune should still set innodb_buffer_pool_instances")
	}
	for _, k := range dbKnobs(true) {
		if removedMariaDBOption(k.Name) {
			t.Fatalf("MariaDB config still offers %s", k.Name)
		}
	}
}

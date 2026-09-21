package software

import "testing"

func TestLatestSeriesVersion(t *testing.T) {
	got := latestSeriesVersion("11.4", []string{"11.4.8", "11.8.3", "11.4.13", "10.11.14"})
	if got != "11.4.13" {
		t.Fatalf("%q", got)
	}
}

func TestParseMariaDBLatest(t *testing.T) {
	body := []byte(`{"releases":{"11.4.8":{},"11.4.13":{},"11.4.2":{}}}`)
	if parseMariaDBLatest(body, "11.4") != "11.4.13" {
		t.Fatal(parseMariaDBLatest(body, "11.4"))
	}
}

func TestParseMySQLTagLatest(t *testing.T) {
	body := []byte(`[{"name":"mysql-8.4.4"},{"name":"mysql-8.0.43"},{"name":"mysql-8.4.6"}]`)
	if parseMySQLTagLatest(body, "8.4") != "8.4.6" {
		t.Fatal(parseMySQLTagLatest(body, "8.4"))
	}
}

func TestSourceURLs(t *testing.T) {
	if mariadbSourceURL("11.4.13") != "https://archive.mariadb.org/mariadb-11.4.13/source/mariadb-11.4.13.tar.gz" {
		t.Fatal(mariadbSourceURL("11.4.13"))
	}
	if mysqlSourceURL("8.4.6") != "https://cdn.mysql.com/Downloads/MySQL-8.4/mysql-boost-8.4.6.tar.gz" {
		t.Fatal(mysqlSourceURL("8.4.6"))
	}
}

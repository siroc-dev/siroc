package dbmgmt

import "testing"

func TestCreatePrivSQL(t *testing.T) {
	if got := CreatePrivSQL("alice_app", "localhost", true); got != "GRANT CREATE ON *.* TO 'alice_app'@'localhost';" {
		t.Fatalf("%s", got)
	}
	if got := CreatePrivSQL("alice_app", "127.0.0.1", false); got != "REVOKE CREATE ON *.* FROM 'alice_app'@'127.0.0.1';" {
		t.Fatalf("%s", got)
	}
	if got := PrefixRevokeSQL("alice_app", "a_b", "localhost"); got != "REVOKE ALL PRIVILEGES ON `a\\_b\\_%`.* FROM 'alice_app'@'localhost';" {
		t.Fatalf("%s", got)
	}
}

func TestLiteralDBName(t *testing.T) {
	name, ok := LiteralDBName("shop")
	if !ok || name != "shop" {
		t.Fatalf("%q %v", name, ok)
	}
	if _, ok := LiteralDBName(`alice\_%`); ok {
		t.Fatal("pattern should not be one database")
	}
	name, ok = LiteralDBName("my_shop")
	if !ok || name != "my_shop" {
		t.Fatalf("%q %v", name, ok)
	}
}

func TestCreatedDatabase(t *testing.T) {
	name, ok := CreatedDatabase("CREATE DATABASE `shop`")
	if !ok || name != "shop" {
		t.Fatalf("%q %v", name, ok)
	}
	name, ok = CreatedDatabase("create schema if not exists otherdb;")
	if !ok || name != "otherdb" {
		t.Fatalf("%q %v", name, ok)
	}
	if _, ok := CreatedDatabase("CREATE TABLE shop.t (id int)"); ok {
		t.Fatal("table")
	}
}

func TestSyncCreatedIgnoresPrefix(t *testing.T) {
	accounts := []SyncAccount{{Username: "alice", Users: []string{"alice_app"}}}
	grants := []SchemaGrant{{DBName: "shop", DBUser: "alice_app"}, {DBName: "bobdb", DBUser: "bob_main"}}
	creators := map[string]string{"custom": "alice_app@localhost", "secret": "root@localhost"}
	have, grant := SyncCreated([]string{"shop", "bobdb", "custom", "secret", "mysql", "notes"}, grants, creators, accounts)
	if len(have) != 1 || have[0].DBName != "shop" || have[0].DBUser != "alice_app" || have[0].Username != "alice" {
		t.Fatalf("have %#v", have)
	}
	if len(grant) != 1 || grant[0].DBName != "custom" || grant[0].DBUser != "alice_app" {
		t.Fatalf("grant %#v", grant)
	}
}

func TestCreatorsFromAudit(t *testing.T) {
	text := "20240101 12:00:00,localhost,alice_app,localhost,10,0,QUERY,test,'CREATE DATABASE `shop`',0\n"
	got := CreatorsFromAudit(text)
	if got["shop"] != "alice_app" {
		t.Fatalf("%#v", got)
	}
}

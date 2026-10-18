package backup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDirectAdminLegacy(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("backup/user.conf", "username=alice\ndomain=shop.test\nemail=alice@shop.test\n")
	write("backup/shop.test.conf", "domain=shop.test\nssl=ON\n")
	write("backup/shop.test.pointers", "www.shop.test\n")
	write("backup/shop.test.subdomains", "blog\n")
	write("domains/shop.test/public_html/index.php", "<?php echo 1;\n")
	write("domains/shop.test/public_html/blog/index.php", "<?php echo 2;\n")
	write("mysql/alice_shop.sql", "CREATE TABLE posts (id INT);\n")
	write("mysql/alice_shop.conf", "user=alice_shop\npasswd=$aes$secret\n")

	plan, err := ParseDirectAdmin(root)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Manifest.Username != "alice" || !plan.Manifest.Account.KeepPassword || !plan.Manifest.Account.SkipFlags {
		t.Fatalf("account: %+v", plan.Manifest.Account)
	}
	if len(plan.Manifest.Sites) != 2 {
		t.Fatalf("sites: %+v", plan.Manifest.Sites)
	}
	if plan.Manifest.Sites[0].Domain != "shop.test" || plan.Manifest.Sites[0].DocRoot != "/home/alice/domains/shop.test/public_html" {
		t.Fatalf("primary: %+v", plan.Manifest.Sites[0])
	}
	if len(plan.Manifest.Sites[0].Aliases) != 1 || plan.Manifest.Sites[0].Aliases[0] != "www.shop.test" {
		t.Fatalf("aliases: %+v", plan.Manifest.Sites[0].Aliases)
	}
	if plan.Manifest.Sites[1].Domain != "blog.shop.test" || plan.Manifest.Sites[1].DocRoot != "/home/alice/domains/shop.test/public_html/blog" {
		t.Fatalf("subdomain: %+v", plan.Manifest.Sites[1])
	}
	if len(plan.Manifest.Databases) != 1 || plan.Manifest.Databases[0].DBName != "alice_shop" || plan.Manifest.Databases[0].DBUser != "alice_shop" {
		t.Fatalf("db: %+v", plan.Manifest.Databases)
	}
	if plan.Manifest.Databases[0].Password == "$aes$secret" || plan.ResetDBPasswords != 1 {
		t.Fatalf("password was not reset: %+v resets=%d", plan.Manifest.Databases[0], plan.ResetDBPasswords)
	}
	if len(plan.Copies) != 1 || plan.Copies[0].Rel != "domains/shop.test" {
		t.Fatalf("copies: %+v", plan.Copies)
	}
}

func TestParseDirectAdminCurrentLayout(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "alice")
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(base, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("user.conf", "username=alice\ndomain=shop.test\n")
	write("domains/shop.test/domain.conf", "domain=shop.test\n")
	write("domains/shop.test/public_html/index.html", "ok")
	write("mysql/alice_shop.conf", "user=alice_shop\npassword=secret12\n")
	write("mysql/alice_shop.sql", "-- dump\n")

	plan, err := ParseDirectAdmin(root)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Manifest.Sites[0].Domain != "shop.test" {
		t.Fatalf("site: %+v", plan.Manifest.Sites)
	}
	if plan.Manifest.Databases[0].Password != "secret12" || plan.ResetDBPasswords != 0 {
		t.Fatalf("kept password: %+v", plan.Manifest.Databases[0])
	}
}

func TestNestedUserArchives(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"user.bob.tar.zst", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "domains"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "domains", "skip.tar.gz"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	got := NestedUserArchives(root)
	if len(got) != 1 || filepath.Base(got[0]) != "user.bob.tar.zst" {
		t.Fatalf("%v", got)
	}
}

func TestParseDirectAdminRejectsOtherArchives(t *testing.T) {
	root := t.TempDir()
	if _, err := ParseDirectAdmin(root); err == nil {
		t.Fatal("expected error")
	}
}

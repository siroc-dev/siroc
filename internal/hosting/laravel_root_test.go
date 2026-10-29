package hosting

import "testing"

func TestPickLaravelRoot(t *testing.T) {
	kind, root, missing := pickLaravelRoot("php", "/home/a/public", "/home/a/apps/shop", true)
	if kind != "laravel" || root != "/home/a/apps/shop" || missing {
		t.Fatalf("chosen %s %s %v", kind, root, missing)
	}
	kind, root, missing = pickLaravelRoot("laravel", "/home/a/app", "/home/a/empty", false)
	if kind != "php" || root != "/home/a/empty" || !missing {
		t.Fatalf("missing artisan %s %s %v", kind, root, missing)
	}
	kind, root, missing = pickLaravelRoot("wordpress", "/home/a/wp", "/home/a/empty", false)
	if kind != "wordpress" || root != "/home/a/wp" || missing {
		t.Fatalf("wordpress %s %s %v", kind, root, missing)
	}
}

func TestUseCustomLaravelRoot(t *testing.T) {
	if !useCustomLaravelRoot("status", "laravel") {
		t.Fatal("laravel status should use the saved path")
	}
	if useCustomLaravelRoot("status", "wordpress") || useCustomLaravelRoot("status", "") {
		t.Fatal("wordpress and plain status stay on the document root")
	}
	if !useCustomLaravelRoot("artisan", "") || !useCustomLaravelRoot("queue", "") {
		t.Fatal("laravel commands should use the saved path")
	}
	if useCustomLaravelRoot("wpcli", "laravel") {
		t.Fatal("wordpress commands stay on the document root")
	}
}

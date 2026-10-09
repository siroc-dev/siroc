package software

import "testing"

func TestRedisSavesDefaultsOn(t *testing.T) {
	if !redisSaves("appendonly no\n") {
		t.Fatal("missing save lines should keep Redis snapshot defaults")
	}
	if redisSaves("save \"\"\nappendonly no\n") {
		t.Fatal("save \"\" should disable snapshots")
	}
	if !redisSaves("save \"\"\nsave 60 1\n") {
		t.Fatal("a later save interval should turn snapshots back on")
	}
}

func TestApplyRedisPersistenceMemoryOnly(t *testing.T) {
	in := "save 900 1\nsave 300 10\nappendonly yes\n"
	out := applyRedisPersistence(in, false, true)
	if redisSaves(out) {
		t.Fatalf("snapshots still on:\n%s", out)
	}
	if !hasDirective(out, `save ""`) {
		t.Fatalf("missing save \"\":\n%s", out)
	}
	if !hasDirective(out, "appendonly no") {
		t.Fatalf("aof still on:\n%s", out)
	}
}

func TestApplyRedisPersistenceRestoresSnapshots(t *testing.T) {
	in := "save \"\"\nappendonly no\n"
	out := applyRedisPersistence(in, true, false)
	if !redisSaves(out) {
		t.Fatalf("snapshots not restored:\n%s", out)
	}
	if !hasDirective(out, "save 900 1") || !hasDirective(out, "appendonly no") {
		t.Fatalf("%s", out)
	}
}

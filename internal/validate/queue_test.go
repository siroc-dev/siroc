package validate

import "testing"

func TestLaravelQueueNames(t *testing.T) {
	got, err := LaravelQueueNames("")
	if err != nil || got != "default" {
		t.Fatalf("empty: %q %v", got, err)
	}
	got, err = LaravelQueueNames(" emails, default, emails ")
	if err != nil || got != "emails,default" {
		t.Fatalf("list: %q %v", got, err)
	}
	if _, err := LaravelQueueNames("bad name"); err == nil {
		t.Fatal("expected invalid")
	}
}

func TestLaravelQueueWorkers(t *testing.T) {
	got, err := LaravelQueueWorkers(0)
	if err != nil || got != 1 {
		t.Fatalf("default: %d %v", got, err)
	}
	got, err = LaravelQueueWorkers(4)
	if err != nil || got != 4 {
		t.Fatalf("ok: %d %v", got, err)
	}
	if _, err := LaravelQueueWorkers(9); err == nil {
		t.Fatal("expected too many workers")
	}
}

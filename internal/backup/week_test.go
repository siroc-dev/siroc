package backup

import (
	"errors"
	"testing"
	"time"
)

func TestWeekdayFolders(t *testing.T) {
	monday := time.Date(2026, 11, 2, 15, 0, 0, 0, time.Local)
	if monday.Weekday() != time.Monday {
		t.Fatal("fixture is not a Monday")
	}
	if got := WeekdayDir(monday); got != "monday" {
		t.Fatalf("monday: %s", got)
	}
	if got := WeekdayDir(monday.AddDate(0, 0, 6)); got != "sunday" {
		t.Fatalf("sunday: %s", got)
	}
	folder, name := WeekRemote("week", monday, "blog", "blog-20261102.tar.gz")
	if folder != "monday" || name != "blog.tar.gz" {
		t.Fatalf("remote %s/%s", folder, name)
	}
	folder, name = WeekRemote("", monday, "blog", "blog-20261102.tar.gz")
	if folder != "" || name != "blog-20261102.tar.gz" {
		t.Fatalf("plain remote %s/%s", folder, name)
	}
	if _, err := NormalizeLayout("nope"); err == nil {
		t.Fatal("accepted invalid layout")
	}
	if got, err := NormalizeLayout("week"); err != nil || got != "week" {
		t.Fatalf("week: %s %v", got, err)
	}
}

func TestCancelRun(t *testing.T) {
	if err := CancelRun(""); err == nil {
		t.Fatal("expected empty id to fail")
	}
	if err := CancelRun("job-early"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := beginRun("job-early"); !errors.Is(err, errCancelled) {
		t.Fatalf("early cancel: %v", err)
	}
	ctx, done, err := beginRun("job-7")
	if err != nil {
		t.Fatal(err)
	}
	if err := CancelRun("job-7"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("cancel did not stop the run")
	}
	done()
}

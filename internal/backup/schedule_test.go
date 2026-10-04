package backup

import "testing"

func TestRenderCronFile(t *testing.T) {
	body, err := RenderCronFile("/usr/local/bin/siroc-panel", []CronTask{
		{ID: 4, Cycle: "daily", Minute: 30, Hour: 1, Enabled: true},
		{ID: 5, Cycle: "hourly", Minute: 15, Enabled: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "30 1 * * * root /usr/local/bin/siroc-panel backup-cron 4 >> /var/log/siroc/backup-cron.log 2>&1\n"
	if !contains(body, want) {
		t.Fatalf("body:\n%s", body)
	}
	if contains(body, "backup-cron 5") {
		t.Fatal("disabled task was written")
	}
	if _, err := RenderCronFile("/tmp/other", []CronTask{{ID: 1, Cycle: "daily", Enabled: true}}); err == nil {
		t.Fatal("expected bad binary")
	}
	if _, err := RenderCronFile("/usr/local/bin/siroc-panel", []CronTask{{ID: 1, Cycle: "daily", Minute: 99, Enabled: true}}); err == nil {
		t.Fatal("expected bad minute")
	}
}

func TestDescribeAndPrune(t *testing.T) {
	got := Describe(CronTask{Cycle: "daily", Hour: 1, Minute: 30})
	if got != "Daily at 01:30" {
		t.Fatal(got)
	}
	drop := PruneNames([]string{"a-3.tar.gz", "a-1.tar.gz", "a-2.tar.gz"}, 2)
	if len(drop) != 1 || drop[0] != "a-1.tar.gz" {
		t.Fatalf("%v", drop)
	}
}

func contains(s, part string) bool {
	return len(part) == 0 || (len(s) >= len(part) && (s == part || indexOf(s, part) >= 0))
}

func indexOf(s, part string) int {
	for i := 0; i+len(part) <= len(s); i++ {
		if s[i:i+len(part)] == part {
			return i
		}
	}
	return -1
}

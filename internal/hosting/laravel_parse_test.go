package hosting

import (
	"testing"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func TestNormalizeQueueSpecs(t *testing.T) {
	rows, err := normalizeQueueSpecs("default, emails", 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Name != "default" || rows[0].Workers != 2 || rows[1].Name != "emails" {
		t.Fatalf("%+v", rows)
	}
	rows, err = normalizeQueueSpecs("", 0, []rpc.LaravelQueueRow{{Name: " emails ", Workers: 3}, {Name: "emails", Workers: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Name != "emails" || rows[0].Workers != 3 {
		t.Fatalf("%+v", rows)
	}
}

func TestParseSupervisorStatusAndUptime(t *testing.T) {
	out := `
siroc-a01-a01_test-q-default:siroc-a01-a01_test-q-default_00   RUNNING   pid 1234, uptime 0:01:23
siroc-a01-a01_test-q-emails:siroc-a01-a01_test-q-emails_00     BACKOFF   can't find command 'php'
`
	list := parseSupervisorStatus(out)
	if len(list) != 2 {
		t.Fatalf("len %d", len(list))
	}
	if list[0].Name != "siroc-a01-a01_test-q-default_00" || list[0].Status != "RUNNING" || list[0].PID != 1234 || list[0].Uptime != "0:01:23" {
		t.Fatalf("%+v", list[0])
	}
	if list[1].Status != "BACKOFF" {
		t.Fatalf("%+v", list[1])
	}
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	last := lastRunFromUptime(now, "0:01:23")
	if last != now.Add(-83*time.Second) {
		t.Fatalf("last %s", last)
	}
	d, ok := parseSupervisorUptime("2 days, 1:00:00")
	if !ok || d != 49*time.Hour {
		t.Fatalf("days %v %v", d, ok)
	}
}

func TestQueueNextRun(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	if queueNextRun("RUNNING", now, now) != "listening" {
		t.Fatal("running")
	}
	got := queueNextRun("BACKOFF", time.Time{}, now)
	if got != now.Add(5*time.Second).UTC().Format(time.RFC3339) {
		t.Fatalf("backoff %s", got)
	}
}

func TestParseScheduleList(t *testing.T) {
	raw := `[{"expression":"* * * * *","command":"inspire","next_due_date":"2026-09-19T12:01:00Z","next_due_human":"in 1 minute"}]`
	got := parseScheduleList(raw)
	if len(got) != 1 || got[0].Command != "inspire" || got[0].NextRun != "2026-09-19T12:01:00Z" {
		t.Fatalf("%+v", got)
	}
	text := `  0 0 * * *  php artisan inspire ........... Next Due: 11 hours from now`
	got = parseScheduleList(text)
	if len(got) != 1 || got[0].Expression != "0 0 * * *" || got[0].Command != "php artisan inspire" || got[0].NextRun != "11 hours from now" {
		t.Fatalf("%+v", got)
	}
}

func TestLaravelQueueNameFromConf(t *testing.T) {
	conf := "command=php artisan queue:work --sleep=3 --queue=emails,default\nnumprocs=2\nstdout_logfile=/tmp/q.log\n"
	if laravelQueueNameFromConf(conf) != "emails,default" {
		t.Fatal(laravelQueueNameFromConf(conf))
	}
	if laravelWorkersFromConf(conf) != 2 {
		t.Fatal(laravelWorkersFromConf(conf))
	}
	if laravelLogFromConf(conf) != "/tmp/q.log" {
		t.Fatal(laravelLogFromConf(conf))
	}
}

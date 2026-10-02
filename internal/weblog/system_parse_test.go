package weblog

import "testing"

func TestParseSystemLogEntriesNewestFirst(t *testing.T) {
	in := `2026-10-02 00:04:00 siroc login failed ip=203.0.113.9 user=admin
2026-10-02 00:05:00 siroc login failed ip=203.0.113.10 user=root
`
	got := ParseSystemLogEntries("panel-auth", "file", in)
	if len(got) != 2 {
		t.Fatalf("entries %d", len(got))
	}
	if got[0].Time != "2026-10-02 00:05:00" || got[0].Level != "error" {
		t.Fatalf("newest %+v", got[0])
	}
	if got[1].Time != "2026-10-02 00:04:00" {
		t.Fatalf("older %+v", got[1])
	}
}

func TestParseSystemSyslogAndJournal(t *testing.T) {
	sys := ParseSystemLogEntries("ssh-auth", "file", `Oct  2 21:00:00 host sshd[12]: Accepted publickey for admin from 10.0.0.2
Oct  2 21:01:00 host sshd[13]: Failed password for root from 10.0.0.9
`)
	if len(sys) != 2 || sys[0].Level != "error" || sys[0].Env != "sshd" || sys[1].Level != "info" {
		t.Fatalf("syslog %+v", sys)
	}
	j := ParseSystemLogEntries("panel-journal", "journal", `2026-10-02T21:00:00+0700 host siroc-panel[1]: Listening on :8080
2026-10-02T21:01:00+0700 host siroc-panel[1]: request failed path=/api/login
`)
	if len(j) != 2 || j[0].Level != "error" || j[0].Env != "siroc-panel" || j[1].Message != "Listening on :8080" {
		t.Fatalf("journal %+v", j)
	}
}

func TestParseSystemFail2banRedisLast(t *testing.T) {
	ban := ParseSystemLogEntries("fail2ban", "file", `2026-10-02 21:00:00,123 fail2ban.actions [1]: NOTICE  [sshd] Ban 1.2.3.4
2026-10-02 21:01:00,123 fail2ban.actions [1]: WARNING [siroc] Ban 9.9.9.9
`)
	if len(ban) != 2 || ban[0].Level != "warning" || ban[1].Level != "notice" {
		t.Fatalf("fail2ban %+v", ban)
	}
	rd := ParseSystemLogEntries("redis", "file", `1234:M 02 Oct 2026 21:00:00.123 * Ready to accept connections
1234:M 02 Oct 2026 21:01:00.123 # WARNING overcommit_memory
`)
	if len(rd) != 2 || rd[0].Level != "warning" || rd[1].Level != "info" {
		t.Fatalf("redis %+v", rd)
	}
	last := ParseSystemLogEntries("ssh-last", "command", `admin    pts/0        10.0.0.2         Fri Oct  2 21:01   still logged in
root     pts/1        10.0.0.3         Fri Oct  2 20:00 - 20:10  (00:10)

wtmp begins Fri Oct  1 00:00:00 2026
`)
	if len(last) != 2 || last[0].Message[:5] != "admin" {
		t.Fatalf("last keeps newest first %+v", last)
	}
}

func TestParseSystemAccessAndPHPFpm(t *testing.T) {
	acc := ParseSystemLogEntries("nginx-access", "file", `127.0.0.1 - - [20/Sep/2026:09:46:00 +0000] "GET / HTTP/1.1" 200 123 "-" "siroc"
10.0.0.2 - - [20/Sep/2026:09:46:01 +0000] "GET /missing HTTP/1.1" 404 12
`)
	if len(acc) != 2 || acc[0].Status != 404 {
		t.Fatalf("access %+v", acc)
	}
	fpm := ParseSystemLogEntries("php-fpm-8.3", "file", `[02-Oct-2026 21:00:00] NOTICE: fpm is running
[02-Oct-2026 21:01:00] ERROR: unable to bind
`)
	if len(fpm) != 2 || fpm[0].Level != "error" || fpm[1].Level != "notice" {
		t.Fatalf("php-fpm %+v", fpm)
	}
}

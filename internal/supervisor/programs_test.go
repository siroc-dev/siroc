package supervisor

import (
	"strings"
	"testing"

	"github.com/siroc-dev/siroc/internal/rpc"
)

const laravelConf = `[program:siroc-abraventure-abraventure-com-q-default]
command=/usr/bin/php artisan queue:work --sleep=3 --tries=3 --timeout=90 --queue=default
directory=/mnt/data/plyxxx
user=abraventure
numprocs=2
process_name=%(program_name)s_%(process_num)02d
autostart=true
autorestart=true
redirect_stderr=true
stdout_logfile=/home/abraventure/tmp/queue-abraventure.com-default.log
stopwaitsecs=30
environment=HOME="/home/abraventure",USER="abraventure"
`

func TestProgramsKeepWebsiteFields(t *testing.T) {
	got := ProgramsIn("/etc/supervisor/conf.d/siroc-abraventure-abraventure-com-q-default.conf", laravelConf)
	if len(got) != 1 {
		t.Fatalf("programs=%d", len(got))
	}
	p := got[0]
	if !p.Site || p.Numprocs != 2 || p.User != "abraventure" || p.Directory != "/mnt/data/plyxxx" {
		t.Fatalf("program=%+v", p)
	}
	if !strings.Contains(p.Command, "--queue=default") {
		t.Fatalf("command=%s", p.Command)
	}
	if p.StdoutLog != "/home/abraventure/tmp/queue-abraventure.com-default.log" {
		t.Fatalf("log=%s", p.StdoutLog)
	}
}

func TestUpdateKeepsExtraLines(t *testing.T) {
	next, remove, err := RewriteConf(laravelConf, "update", rpc.SupervisorProgram{
		Name:        "siroc-abraventure-abraventure-com-q-default",
		Command:     "/usr/bin/php artisan queue:work --queue=mail",
		Directory:   "/mnt/data/plyxxx",
		User:        "abraventure",
		Numprocs:    2,
		Autostart:   true,
		Autorestart: true,
		StdoutLog:   "/home/abraventure/tmp/queue-abraventure.com-default.log",
	})
	if err != nil || remove {
		t.Fatalf("rewrite err=%v remove=%v", err, remove)
	}
	if !strings.Contains(next, "--queue=mail") {
		t.Fatalf("command not updated:\n%s", next)
	}
	for _, want := range []string{"process_name=", "environment=HOME=", "stopwaitsecs=30", "redirect_stderr=true"} {
		if !strings.Contains(next, want) {
			t.Fatalf("missing %s\n%s", want, next)
		}
	}
	other := "[program:keepme]\ncommand=/bin/true\n"
	file := laravelConf + "\n" + other
	next, remove, err = RewriteConf(file, "delete", rpc.SupervisorProgram{Name: "siroc-abraventure-abraventure-com-q-default"})
	if err != nil || remove {
		t.Fatalf("delete err=%v remove=%v", err, remove)
	}
	if strings.Contains(next, "siroc-abraventure") || !strings.Contains(next, "keepme") {
		t.Fatalf("delete result:\n%s", next)
	}
	empty, remove, err := RewriteConf(other, "delete", rpc.SupervisorProgram{Name: "keepme"})
	if err != nil || !remove || empty != "" {
		t.Fatalf("empty=%q remove=%v err=%v", empty, remove, err)
	}
}

func TestPrepareRejectsWebsiteNames(t *testing.T) {
	_, err := Prepare(rpc.SupervisorProgramReq{Name: "siroc-queue", Command: "/bin/true"}, nil)
	if err == nil || !strings.Contains(err.Error(), "siroc-") {
		t.Fatalf("err=%v", err)
	}
	got, err := Prepare(rpc.SupervisorProgramReq{
		Name: "worker", Command: "/usr/bin/php /var/www/worker.php",
		Directory: "/var/www", User: "www-data", Numprocs: 2, Autostart: true, Autorestart: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.StdoutLog != "/var/log/supervisor/worker.log" || got.Numprocs != 2 || got.User != "www-data" {
		t.Fatalf("%+v", got)
	}
	_, err = Prepare(rpc.SupervisorProgramReq{Name: "worker", Command: "/bin/true", Directory: "/var/../etc"}, nil)
	if err == nil {
		t.Fatal("expected directory rejection")
	}
	_, err = Prepare(rpc.SupervisorProgramReq{Name: "ok", Command: "/bin/true"}, &rpc.SupervisorProgram{Name: "siroc-queue", Site: true})
	if err == nil {
		t.Fatal("expected website edit rejection")
	}
}

func TestStatusKeepsProcessPrefix(t *testing.T) {
	conf := ProgramsIn("/etc/supervisor/conf.d/worker.conf", "[program:worker]\ncommand=/bin/true\nnumprocs=2\n")
	status := "worker:worker_00                 RUNNING   pid 7, uptime 1 day, 0:00:05\n" +
		"worker:worker_01                 FATAL     Exited too quickly\n" +
		"orphan                           STOPPED   Not started\n"
	got := ApplyStatus(conf, status)
	if len(got) != 2 {
		t.Fatalf("rows=%d %+v", len(got), got)
	}
	var worker rpc.SupervisorProgram
	for _, row := range got {
		if row.Name == "worker" {
			worker = row
		}
		if row.Name == "worker_00" || row.Name == "worker_01" {
			t.Fatalf("process suffix became a program: %+v", got)
		}
	}
	if len(worker.Processes) != 2 || worker.Processes[0].Name != "worker:worker_00" || worker.Processes[0].PID != 7 {
		t.Fatalf("worker=%+v", worker)
	}
	if worker.Processes[0].Uptime != "1 day, 0:00:05" {
		t.Fatalf("uptime=%q", worker.Processes[0].Uptime)
	}
}

func TestAllowedLog(t *testing.T) {
	if !AllowedLog("/home/abraventure/tmp/queue.log") || !AllowedLog("/var/log/supervisor/worker.log") {
		t.Fatal("expected allowed logs")
	}
	if AllowedLog("/etc/passwd") || AllowedLog("/var/log/../etc/passwd") || AllowedLog("relative.log") {
		t.Fatal("expected rejected logs")
	}
}

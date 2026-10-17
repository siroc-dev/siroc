package rclone

import "testing"

func TestInterpretStats(t *testing.T) {
	line := `{"level":"info","msg":"stats","stats":{"bytes":500,"totalBytes":1000,"speed":1200,"transfers":1,"totalTransfers":4,"transferring":[{"name":"dir/file.bin","size":800,"bytes":200,"percentage":0,"speed":50}]}}`
	log, prog := Interpret(line)
	if log != "" || prog == nil {
		t.Fatalf("log %q prog %#v", log, prog)
	}
	if prog.Percent != 50 || prog.Bytes != 500 || prog.TotalBytes != 1000 || prog.Speed != 1200 {
		t.Fatalf("overall %#v", prog)
	}
	if len(prog.Files) != 1 || prog.Files[0].Name != "dir/file.bin" || prog.Files[0].Percent != 25 {
		t.Fatalf("file %#v", prog.Files)
	}
}

func TestInterpretPartialTotals(t *testing.T) {
	line := `{"level":"info","msg":"stats","stats":{"bytes":0,"totalBytes":0,"transfers":2,"totalTransfers":10,"listed":40,"transferring":[{"name":"a.bin","size":0,"bytes":0,"percentage":12.5}]}}`
	_, prog := Interpret(line)
	if prog == nil || prog.Percent != 20 || prog.Listed != 40 || prog.Transfers != 2 || prog.TotalTransfers != 10 {
		t.Fatalf("%#v", prog)
	}
	if len(prog.Files) != 1 || prog.Files[0].Percent != 12 {
		t.Fatalf("file %#v", prog.Files)
	}
}

func TestInterpretLogLine(t *testing.T) {
	log, prog := Interpret(`{"level":"error","msg":"failed to copy file.bin"}`)
	if prog != nil || log != "failed to copy file.bin" {
		t.Fatalf("log %q prog %#v", log, prog)
	}
	log, prog = Interpret("plain output")
	if prog != nil || log != "plain output" {
		t.Fatalf("log %q prog %#v", log, prog)
	}
	log, prog = Interpret("  ")
	if prog != nil || log != "" {
		t.Fatalf("blank line should be ignored, log %q", log)
	}
}

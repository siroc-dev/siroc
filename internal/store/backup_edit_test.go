package store

import (
	"path/filepath"
	"testing"
)

func TestUpdateBackupDestAndCron(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	dest, err := st.CreateBackupDest(BackupDest{Name: "Offsite", Kind: "ftp", Host: "ftp.example", Port: 21, User: "alice", Path: "/backups"})
	if err != nil {
		t.Fatal(err)
	}
	dest.Name = "Offsite 2"
	dest.Host = "files.example"
	if err := st.UpdateBackupDest(*dest); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetBackupDest(dest.ID)
	if err != nil || got.Name != "Offsite 2" || got.Host != "files.example" {
		t.Fatalf("%+v %v", got, err)
	}
	cron, err := st.CreateBackupCron(BackupCron{Name: "Nightly", Account: "*", DestID: dest.ID, IncludeDB: true, Cycle: "daily", Hour: 1, Minute: 30, Retain: 3, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	cron.Name = "Weekly"
	cron.Cycle = "weekly"
	cron.Weekday = 1
	cron.Retain = 5
	if err := st.UpdateBackupCron(*cron); err != nil {
		t.Fatal(err)
	}
	updated, err := st.GetBackupCron(cron.ID)
	if err != nil || updated.Name != "Weekly" || updated.Cycle != "weekly" || updated.Retain != 5 || !updated.Enabled {
		t.Fatalf("%+v %v", updated, err)
	}
	if err := st.UpdateBackupDest(*got); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateBackupCron(*updated); err != nil {
		t.Fatal(err)
	}
}

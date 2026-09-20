package store

import (
	"path/filepath"
	"testing"
)

func TestLastInstallStatus(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.LastInstallStatus("php", "8.4")
	if err != nil || got != "" {
		t.Fatalf("empty: %q %v", got, err)
	}
	if _, err := st.EnqueueInstall("php", "8.4"); err != nil {
		t.Fatal(err)
	}
	job, err := st.EnqueueInstall("php", "8.4")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.FinishInstall(job.ID, "error", "a2enmod missing"); err != nil {
		t.Fatal(err)
	}
	got, err = st.LastInstallStatus("php", "8.4")
	if err != nil || got != "error" {
		t.Fatalf("got %q %v", got, err)
	}
}

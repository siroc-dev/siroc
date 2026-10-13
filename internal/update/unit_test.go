package update

import (
	"strings"
	"testing"
)

func TestStripRuntimeDirectoryGroup(t *testing.T) {
	in := "[Service]\nRuntimeDirectory=siroc\nRuntimeDirectoryMode=0750\nRuntimeDirectoryGroup=siroc\nExecStart=/usr/local/bin/siroc-agent\n"
	got, changed := StripRuntimeDirectoryGroup(in)
	if !changed {
		t.Fatal("expected a change")
	}
	if strings.Contains(got, "RuntimeDirectoryGroup=") {
		t.Fatalf("key still present:\n%s", got)
	}
	if !strings.Contains(got, "RuntimeDirectory=siroc\n") || !strings.Contains(got, "ExecStart=/usr/local/bin/siroc-agent\n") {
		t.Fatalf("unit damaged:\n%s", got)
	}
	again, changed := StripRuntimeDirectoryGroup(got)
	if changed || again != got {
		t.Fatal("second pass should be a no-op")
	}
}

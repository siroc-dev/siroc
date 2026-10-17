package software

import "strings"
import "testing"

func TestCLILimitsINI(t *testing.T) {
	got := CLILimitsINI()
	if !strings.Contains(got, "max_execution_time = 0\n") {
		t.Fatalf("%q", got)
	}
	if !strings.Contains(got, "memory_limit = -1\n") {
		t.Fatalf("%q", got)
	}
}

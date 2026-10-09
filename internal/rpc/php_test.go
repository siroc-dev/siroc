package rpc

import (
	"strings"
	"testing"
)

func TestDefaultPHPFPMAllowsProcOpen(t *testing.T) {
	got := DefaultPHPFPM().DisableFunctions
	if got != defaultDisableFunctions || strings.Contains(got, "proc_open") {
		t.Fatalf("default disable_functions = %q", got)
	}
}

func TestMergePHPFPMDropsLegacyProcOpen(t *testing.T) {
	got := MergePHPFPM(PHPFPMSettings{DisableFunctions: legacyDisableFunctions})
	if got.DisableFunctions != defaultDisableFunctions {
		t.Fatalf("got %q", got.DisableFunctions)
	}
	custom := "exec,proc_open"
	got = MergePHPFPM(PHPFPMSettings{DisableFunctions: custom})
	if got.DisableFunctions != custom {
		t.Fatalf("custom list changed to %q", got.DisableFunctions)
	}
}

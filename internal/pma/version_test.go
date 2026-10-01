package pma

import "testing"

func TestPickPHPVersion(t *testing.T) {
	if got := pickPHPVersion([]string{"8.3"}, []string{"8.4", "8.3"}); got != "8.3" {
		t.Fatalf("prefer running FPM, got %q", got)
	}
	if got := pickPHPVersion(nil, []string{"8.2"}); got != "8.2" {
		t.Fatalf("installed %q", got)
	}
	if got := pickPHPVersion(nil, nil); got != "" {
		t.Fatalf("empty %q", got)
	}
}

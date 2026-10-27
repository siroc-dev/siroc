//go:build linux

package files

import (
	"strings"
	"testing"
)

func TestTarExtractArgs(t *testing.T) {
	got := tarExtractArgs("site.tar.gz", "/tmp/out")
	if len(got) != 4 || got[0] != "-xzf" || got[1] != "site.tar.gz" || got[2] != "-C" || got[3] != "/tmp/out" {
		t.Fatalf("%v", got)
	}
	for _, a := range got {
		if strings.HasPrefix(a, "--") {
			t.Fatalf("busybox tar rejects %s", a)
		}
	}
}

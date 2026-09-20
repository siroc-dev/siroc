package update

import "testing"

func TestResolveChannel(t *testing.T) {
	if ResolveChannel("") != DefaultChannel {
		t.Fatal("empty should default")
	}
	if ResolveChannel(" https://get.siroc.dev/ ") != DefaultChannel {
		t.Fatal("trim slash")
	}
	if ResolveChannel("https://example.test/updates") != "https://example.test/updates" {
		t.Fatal("custom channel")
	}
}

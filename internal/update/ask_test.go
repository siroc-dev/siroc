package update

import "testing"

func TestAccepted(t *testing.T) {
	for _, answer := range []string{"y", "Y", "yes", " YES "} {
		if !Accepted(answer) {
			t.Fatalf("expected accept %q", answer)
		}
	}
	for _, answer := range []string{"", "n", "no", "maybe"} {
		if Accepted(answer) {
			t.Fatalf("expected decline %q", answer)
		}
	}
}

func TestOfferInstall(t *testing.T) {
	if !OfferInstall(true, true, false) {
		t.Fatal("interactive update should prompt")
	}
	if OfferInstall(true, true, true) || OfferInstall(true, false, false) || OfferInstall(false, true, false) {
		t.Fatal("prompt should stay off for --apply, a non-terminal, or no update")
	}
}

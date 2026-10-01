package secret

import (
	"testing"

	"github.com/siroc-dev/siroc/internal/validate"
)

func TestRandomIdent(t *testing.T) {
	got, err := RandomIdent(6)
	if err != nil {
		t.Fatal(err)
	}
	if err := validate.DBIdent(got); err != nil {
		t.Fatalf("%q: %v", got, err)
	}
	pw, err := RandomPassword(16)
	if err != nil || len(pw) < 8 {
		t.Fatalf("password %q %v", pw, err)
	}
}

package security

import (
	"testing"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func TestOrderWAFRulesPutsPinnedFirst(t *testing.T) {
	rules := []rpc.WAFRule{{ID: 911100}, {ID: 920100}, {ID: 941100}, {ID: 942100}}
	got := orderWAFRules(rules, []int{942100, 941100}, 3)
	want := []int{941100, 942100, 911100}
	if len(got) != len(want) {
		t.Fatalf("len %d", len(got))
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("got %v want %v", ids(got), want)
		}
	}
}

func TestParseWAFPin(t *testing.T) {
	got := ParseWAFPin("942100, no, 12, 941100")
	if len(got) != 2 || got[0] != 942100 || got[1] != 941100 {
		t.Fatalf("%v", got)
	}
}

func ids(rules []rpc.WAFRule) []int {
	out := make([]int, len(rules))
	for i, r := range rules {
		out[i] = r.ID
	}
	return out
}

package software

import "testing"

func TestBaseStack(t *testing.T) {
	got := BaseStack()
	if len(got) != 5 {
		t.Fatalf("len %d", len(got))
	}
	if got[0].Name != "nginx" || got[1].Name != "apache" || got[2].Name != "php" || got[2].Version != "8.4" {
		t.Fatalf("%+v", got)
	}
	if got[3].Name != "mariadb" || got[3].Version != "11.4" {
		t.Fatalf("mariadb %+v", got[3])
	}
	if got[4].Name != "git" {
		t.Fatalf("git %+v", got[4])
	}
}

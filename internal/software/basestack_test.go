package software

import "testing"

func TestBaseStack(t *testing.T) {
	got := BaseStack()
	if len(got) != 7 {
		t.Fatalf("len %d", len(got))
	}
	if got[0].Name != "nginx" || got[1].Name != "apache" || got[2].Name != "php" || got[2].Version != "8.4" {
		t.Fatalf("%+v", got)
	}
	if got[3].Name != "mariadb" || got[3].Version != "11.4" {
		t.Fatalf("mariadb %+v", got[3])
	}
	if got[4].Name != "git" || got[5].Name != "ufw" || got[6].Name != "fail2ban" {
		t.Fatalf("tail %+v", got[4:])
	}
}

func TestRecommendedInstall(t *testing.T) {
	maria, err := RecommendedInstall("")
	if err != nil {
		t.Fatal(err)
	}
	mysql, err := RecommendedInstall("mysql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RecommendedInstall("both"); err == nil {
		t.Fatal("expected error")
	}
	want := []string{"nginx", "apache", "mariadb", "redis", "certbot", "vsftpd"}
	if len(maria) != len(want) {
		t.Fatalf("len %d", len(maria))
	}
	for i, name := range want {
		if maria[i].Name != name {
			t.Fatalf("maria[%d]=%s", i, maria[i].Name)
		}
	}
	if maria[2].Version != "11.4" || mysql[2].Name != "mysql" || mysql[2].Version != "8.4" {
		t.Fatalf("db maria=%+v mysql=%+v", maria[2], mysql[2])
	}
	for _, pkg := range mysql {
		if pkg.Name == "mariadb" {
			t.Fatal("mysql choice also queued mariadb")
		}
	}
}

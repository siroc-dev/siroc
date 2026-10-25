package migrate

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func TestParseEndpoint(t *testing.T) {
	got, err := ParseEndpoint("203.0.113.10", 0)
	if err != nil || got != "https://203.0.113.10:2222" {
		t.Fatalf("default port: %s %v", got, err)
	}
	got, err = ParseEndpoint("https://panel.example:2222/CMD_LOGIN", 0)
	if err != nil || got != "https://panel.example:2222" {
		t.Fatalf("url: %s %v", got, err)
	}
	if _, err := ParseEndpoint("", 2222); err == nil {
		t.Fatal("empty host")
	}
}

func TestDomainAndUserNames(t *testing.T) {
	v, err := url.ParseQuery("error=0&list[]=alice&list[]=bob")
	if err != nil {
		t.Fatal(err)
	}
	users := UserNames(v)
	if len(users) != 2 || users[0] != "alice" || users[1] != "bob" {
		t.Fatalf("users: %#v", users)
	}
	v, err = url.ParseQuery("error=0&shop.test=shop.test&blog.shop.test=blog.shop.test&text=ok")
	if err != nil {
		t.Fatal(err)
	}
	domains := DomainNames(v)
	if len(domains) != 2 || domains[0] != "blog.shop.test" || domains[1] != "shop.test" {
		t.Fatalf("domains: %#v", domains)
	}
	if DAError(v) != "" {
		t.Fatal(DAError(v))
	}
	bad, _ := url.ParseQuery("error=1&text=Login+failed")
	if DAError(bad) != "Login failed" {
		t.Fatal(DAError(bad))
	}
}

func TestNewStableArchive(t *testing.T) {
	before := map[string]string{"old.tar.gz": "10"}
	now := []fileEntry{{name: "old.tar.gz", size: "10"}, {name: "new.tar.gz", size: "4"}}
	name, prev := NewStableArchive(before, nil, now)
	if name != "" || prev["new.tar.gz"] != "4" {
		t.Fatalf("first look: %s %#v", name, prev)
	}
	name, _ = NewStableArchive(before, prev, now)
	if name != "new.tar.gz" {
		t.Fatalf("stable: %s", name)
	}
	growing := []fileEntry{{name: "new.tar.gz", size: "8"}}
	name, prev = NewStableArchive(before, prev, growing)
	if name != "" {
		t.Fatalf("still growing: %s", name)
	}
	name, _ = NewStableArchive(before, prev, growing)
	if name != "new.tar.gz" {
		t.Fatalf("grown stable: %s", name)
	}
}

func TestDiscoverUsers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || pass != "secret" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/CMD_API_SHOW_USERS":
			if user != "admin" {
				t.Errorf("user %s", user)
			}
			_, _ = w.Write([]byte("error=0&list[]=alice"))
		case r.URL.Path == "/CMD_API_SHOW_USER_DOMAINS":
			if user != "admin|alice" {
				t.Errorf("impersonate %s", user)
			}
			_, _ = w.Write([]byte("error=0&shop.test=shop.test"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := &Client{Base: srv.URL, User: "admin", Pass: "secret", http: srv.Client()}
	sites, err := c.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 1 || sites[0].Domain != "shop.test" || sites[0].Owner != "alice" {
		t.Fatalf("%#v", sites)
	}
	if _, err := Connect(rpc.DAConn{}); err == nil {
		t.Fatal("empty connection")
	}
}

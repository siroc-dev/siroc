package store

import (
	"path/filepath"
	"testing"
)

func TestSiteGitUpsertAndToken(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	acc, err := st.CreateAccount("git01", 1101, 1101, "", true, true)
	if err != nil {
		t.Fatal(err)
	}
	site, err := st.CreateSite(acc.ID, "git01.test", "/home/git01/domains/git01.test/public_html", "8.4", nil, "php", "", "", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	g := &SiteGit{
		SiteID:  site.ID,
		Repo:    "git@github.com:org/app.git",
		Branch:  "main",
		Path:    "domains/git01.test",
		Command: LaravelHint(),
		Token:   "tok-aaa",
	}
	if err := st.UpsertSiteGit(g); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetSiteGit(site.ID)
	if err != nil || got.Repo != g.Repo || got.Token != "tok-aaa" || got.Path != "domains/git01.test" {
		t.Fatalf("get: %+v %v", got, err)
	}
	byTok, err := st.GetSiteGitByToken("tok-aaa")
	if err != nil || byTok.SiteID != site.ID {
		t.Fatalf("token: %+v %v", byTok, err)
	}
	if err := st.UpdateSiteGitResult(site.ID, true, "ok"); err != nil {
		t.Fatal(err)
	}
	got, err = st.GetSiteGit(site.ID)
	if err != nil || !got.LastOK || got.LastLog != "ok" || got.LastAt == "" {
		t.Fatalf("result: %+v %v", got, err)
	}
	if err := st.DeleteSite(site.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetSiteGit(site.ID); err == nil {
		t.Fatal("site_git should be removed with the site")
	}
}

func LaravelHint() string {
	return "composer install --no-dev"
}

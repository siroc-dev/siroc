package software

import (
	"strings"
	"testing"
)

func TestParseAndReplaceDebLine(t *testing.T) {
	line := "deb [signed-by=/etc/apt/keyrings/cp-mariadb.gpg] https://deb.mariadb.org/11.4/ubuntu resolute main"
	prefix, url, suite, rest, ok := parseDebLine(line)
	if !ok || prefix != "deb [signed-by=/etc/apt/keyrings/cp-mariadb.gpg]" || url != "https://deb.mariadb.org/11.4/ubuntu" || suite != "resolute" || rest != "main" {
		t.Fatalf("%q %q %q %q %v", prefix, url, suite, rest, ok)
	}
	got := replaceDebSuite(line, "noble")
	want := "deb [signed-by=/etc/apt/keyrings/cp-mariadb.gpg] https://deb.mariadb.org/11.4/ubuntu noble main"
	if got != want {
		t.Fatalf("got %q", got)
	}
	if _, _, _, _, ok := parseDebLine("# deb http://example resolute main"); ok {
		t.Fatal("comment should not parse")
	}
}

func TestFallbackSuites(t *testing.T) {
	got := fallbackSuites("ubuntu", "resolute")
	if got[0] != "resolute" || got[1] != "noble" {
		t.Fatalf("%v", got)
	}
	if !officialArchive("http://nova.clouds.archive.ubuntu.com/ubuntu") || !officialArchive("https://security.ubuntu.com/ubuntu") {
		t.Fatal("official archive")
	}
	if officialArchive("https://ppa.launchpadcontent.net/ondrej/php/ubuntu") {
		t.Fatal("ondrej ppa is third-party")
	}
	if !launchpadPPA("https://ppa.launchpadcontent.net/ondrej/php/ubuntu") {
		t.Fatal("launchpad ppa")
	}
}

func TestReleaseURLs(t *testing.T) {
	u := releaseURLs("https://deb.mariadb.org/11.4/ubuntu", "resolute")
	if u[0] != "https://deb.mariadb.org/11.4/ubuntu/dists/resolute/InRelease" {
		t.Fatal(u)
	}
}

func TestPickRepoSuite(t *testing.T) {
	probe := func(_ string, suite string) repoProbe {
		switch suite {
		case "resolute":
			return repoProbeMissing
		case "noble":
			return repoProbeOK
		default:
			return repoProbeMissing
		}
	}
	suite, disable := pickRepoSuite("https://deb.mariadb.org/11.4/ubuntu", "ubuntu", "resolute", probe)
	if disable || suite != "noble" {
		t.Fatalf("fallback %q %v", suite, disable)
	}
	allUnknown := func(string, string) repoProbe { return repoProbeUnknown }
	suite, disable = pickRepoSuite("https://nginx.org/packages/mainline/ubuntu", "ubuntu", "resolute", allUnknown)
	if disable || suite != "resolute" {
		t.Fatalf("keep current when unknown %q %v", suite, disable)
	}
	allMissing := func(string, string) repoProbe { return repoProbeMissing }
	suite, disable = pickRepoSuite("https://deb.mariadb.org/11.4/ubuntu", "ubuntu", "resolute", allMissing)
	if !disable || suite != "" {
		t.Fatalf("disable when every suite 404 %q %v", suite, disable)
	}
	ondrej := "https://ppa.launchpadcontent.net/ondrej/php/ubuntu"
	suite, disable = pickRepoSuite(ondrej, "ubuntu", "resolute", probe)
	if !disable || suite != "" {
		t.Fatalf("ondrej must not fall back to noble on resolute %q %v", suite, disable)
	}
	sury := "https://packages.sury.org/php"
	suite, disable = pickRepoSuite(sury, "ubuntu", "resolute", probe)
	if !disable || suite != "" {
		t.Fatalf("sury must not fall back to noble on resolute %q %v", suite, disable)
	}
	suryOK := func(_ string, suite string) repoProbe {
		if suite == "resolute" {
			return repoProbeOK
		}
		return repoProbeMissing
	}
	suite, disable = pickRepoSuite(sury, "ubuntu", "resolute", suryOK)
	if disable || suite != "resolute" {
		t.Fatalf("sury resolute %q %v", suite, disable)
	}
}

func TestDeb822SuiteRewrite(t *testing.T) {
	block := "Types: deb\nURIs: https://ppa.launchpadcontent.net/ondrej/php/ubuntu\nSuites: resolute\nComponents: main\n"
	if firstField(deb822Field(block, "URIs")) != "https://ppa.launchpadcontent.net/ondrej/php/ubuntu" {
		t.Fatal("uri")
	}
	got := replaceDeb822Field(block, "Suites", "noble")
	if deb822Field(got, "Suites") != "noble" {
		t.Fatalf("%q", got)
	}
	off := replaceDeb822Field(block, "Enabled", "no")
	if deb822Field(off, "Enabled") != "no" {
		t.Fatal("disable")
	}
	// Appending a missing field must stay inside the stanza, not land after a
	// trailing blank line where it becomes a phantom stanza (apt "Malformed
	// stanza" error).
	trailing := "Types: deb\nURIs: https://deb.nodesource.com/node_22.x\nSuites: nodistro\n\n"
	got2 := replaceDeb822Field(trailing, "Enabled", "no")
	nonEmpty := 0
	for _, b := range splitDeb822(got2) {
		if strings.TrimSpace(b) != "" {
			nonEmpty++
		}
	}
	if nonEmpty != 1 {
		t.Fatalf("append leaked into a second stanza: %q", got2)
	}
	if deb822Field(got2, "Enabled") != "no" {
		t.Fatalf("enabled not set: %q", got2)
	}
}

func TestDeb822Junk(t *testing.T) {
	// A bare "Enabled: no" with no Types/URIs is a malformed stanza and must
	// be flagged as junk; valid and comment-only blocks must not be.
	if !deb822Junk("Enabled: no") {
		t.Fatal("lone Enabled must be junk")
	}
	if deb822Junk("Types: deb\nURIs: https://x\nSuites: noble\nEnabled: no") {
		t.Fatal("valid stanza must not be junk")
	}
	if deb822Junk("# just a comment\n# another") {
		t.Fatal("comment-only block must not be junk")
	}
	if deb822Junk("deb http://example noble main") {
		t.Fatal("one-line style must not be junk")
	}
}

func TestMySQLPackageComponent(t *testing.T) {
	body := "-----BEGIN PGP SIGNED MESSAGE-----\nHash: SHA256\n\nComponents: mysql-8.0 mysql-8.4-lts mysql-9.7-lts mysql-tools\n"
	have := parseReleaseComponents(body)
	if c, ok := pickMySQLComponent("8.4", have); !ok || c != "mysql-8.4-lts" {
		t.Fatalf("8.4 %q %v", c, ok)
	}
	if c, ok := pickMySQLComponent("9.7", have); !ok || c != "mysql-9.7-lts" {
		t.Fatalf("9.7 %q %v", c, ok)
	}
	if _, ok := pickMySQLComponent("8.0", []string{"mysql-8.4-lts", "mysql-9.7-lts"}); ok {
		t.Fatal("8.0 should be missing on a repo that does not publish it")
	}
	if parseReleaseComponents("-----BEGIN PGP SIGNATURE-----\nComponents: mysql-8.0\n") != nil {
		t.Fatal("signature block")
	}
}

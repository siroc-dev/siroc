package software

import "testing"

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
	if !managedRepoList("cp-mariadb.list") || !managedRepoList("mariadb.list") || managedRepoList("ubuntu.sources") {
		t.Fatal("managed repo filter")
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
}

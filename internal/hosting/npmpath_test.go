package hosting

import "strings"
import "testing"

func TestWrapNpmBins(t *testing.T) {
	got := wrapNpmBins("npm install\nnpm run build\n")
	if strings.Count(got, "chmod a+x") != 2 {
		t.Fatalf("want chmod before the command and after npm install\n%s", got)
	}
	if !strings.Contains(got, "npm run build") {
		t.Fatal(got)
	}
	once := wrapNpmBins("npm run build")
	if strings.Count(once, "chmod a+x") != 1 {
		t.Fatalf("build-only should chmod once\n%s", once)
	}
}

package hosting

import "strings"
import "testing"

func TestWrapNpmBins(t *testing.T) {
	got := wrapNpmBins("npm install\nnpm run build\n")
	if strings.Count(got, "if [ -d node_modules/.bin ]") != 2 {
		t.Fatalf("want chmod before the command and after npm install\n%s", got)
	}
	if !strings.Contains(got, "npm run build") || !strings.Contains(got, "readlink -f") {
		t.Fatal(got)
	}
	once := wrapNpmBins("npm run build")
	if strings.Count(once, "if [ -d node_modules/.bin ]") != 1 {
		t.Fatalf("build-only should chmod once\n%s", once)
	}
	prefixed := wrapNpmBins("'/home/site/.local/bin/npm' install\n'/home/site/.local/bin/npm' run build")
	if strings.Count(prefixed, "if [ -d node_modules/.bin ]") != 2 {
		t.Fatalf("a quoted npm path should still chmod after install\n%s", prefixed)
	}
}

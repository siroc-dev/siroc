package hosting

import "strings"
import "testing"

func TestCommandErrorLine(t *testing.T) {
	log := "vite v8.0.3 building client environment for production...\n" +
		"error during build:\n" +
		"Build failed with 1 error:\n" +
		"[UNRESOLVED_ENTRY] Error: Cannot resolve entry module index.html.\n" +
		"    at aggregateBindingErrorsIntoJsError (file:///home/movieovo/domains/movie.rdp-service.com/public_html/node_modules/@voidzero-dev/vite-plus-core/dist/rolldown/shared/error.mjs:48:18)\n" +
		"voidzero-dev/vite-plus-core/dist/rolldown/shared/rolldown-Bblizyw1.mjs:132:34) at async buildEnvironment (file:///home/movieovo/domains/movie.rdp-service.com/public_html/node_modules/@voidzero-dev/vite-plus-core/dist/vite/node/chunks/node.js:39172:66) { errors: [Getter/Setter] }\n"
	got := commandErrorLine(log)
	if got != "[UNRESOLVED_ENTRY] Error: Cannot resolve entry module index.html." {
		t.Fatal(got)
	}
}

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

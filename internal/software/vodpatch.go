package software

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// vodCompilerOpt is the nginx-vod-module recommendation: -O3, popcnt, 256 tracks, AVX2.
// popcnt and AVX2 are included only when the CPU flags say the server can run them.
func vodCompilerOpt(cpu string) string {
	cpu = " " + strings.ToLower(cpu) + " "
	parts := []string{"-O3", "-DNGX_VOD_MAX_TRACK_COUNT=256", "-Wno-error=deprecated-declarations"}
	if strings.Contains(cpu, " popcnt ") {
		parts = append(parts, "-mpopcnt")
	}
	if strings.Contains(cpu, " avx2 ") {
		parts = append(parts, "-mavx2")
	}
	return strings.Join(parts, " ")
}

// patchVodSource fixes nginx-vod-module 1.33 on GCC 15. An empty parameter
// list is void(void) in C23, but Nginx's exit_master expects ngx_cycle_t *.
func patchVodSource(src string) (string, error) {
	const oldDecl = "static void ngx_http_vod_exit_process();"
	const newDecl = "static void ngx_http_vod_exit_process(ngx_cycle_t *cycle);"
	const oldDef = "ngx_http_vod_exit_process()"
	const newDef = "ngx_http_vod_exit_process(ngx_cycle_t *cycle)"
	if strings.Contains(src, newDecl) && strings.Contains(src, "(void)cycle;") && !strings.Contains(src, oldDecl) {
		return src, nil
	}
	if !strings.Contains(src, oldDecl) {
		return "", fmt.Errorf("nginx-vod-module is missing ngx_http_vod_exit_process(); the compatibility patch needs an update")
	}
	src = strings.Replace(src, oldDecl, newDecl, 1)
	if !strings.Contains(src, oldDef) {
		return "", fmt.Errorf("nginx-vod-module exit_process definition changed")
	}
	src = strings.Replace(src, oldDef, newDef, 1)
	pos := strings.LastIndex(src, newDef)
	rest := src[pos+len(newDef):]
	brace := strings.Index(rest, "{")
	if brace < 0 {
		return "", fmt.Errorf("nginx-vod-module exit_process body is missing")
	}
	at := pos + len(newDef) + brace + 1
	if !strings.Contains(src[pos:], "(void)cycle;") {
		src = src[:at] + "\n    (void)cycle;" + src[at:]
	}
	return src, nil
}

// findNamedFile returns the first path under root whose base name matches.
// The vod tarball extracts to nginx-build/vod, and dfxp_format.c lives at
// vod/subtitle/dfxp_format.c inside that tree, not beside ngx_http_vod_module.c.
func findNamedFile(root, name string) (string, error) {
	var found string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != name {
			return nil
		}
		found = path
		return fs.SkipAll
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("%s not found under %s", name, root)
	}
	return found, nil
}

// patchVodDFXP drops libxml2 2.14's deprecated ctxt->recovery check.
// Nginx builds with -Werror, so that warning stops the VOD module compile.
func patchVodDFXP(src string) (string, error) {
	const old = "(!ctxt->wellFormed && !ctxt->recovery))"
	const next = "!ctxt->wellFormed)"
	if !strings.Contains(src, "ctxt->recovery") {
		return src, nil
	}
	if !strings.Contains(src, old) {
		return "", fmt.Errorf("nginx-vod-module dfxp recovery check changed")
	}
	return strings.ReplaceAll(src, old, next), nil
}

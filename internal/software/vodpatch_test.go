package software

import "testing"

func TestVodCompilerOpt(t *testing.T) {
	got := vodCompilerOpt("fpu sse popcnt avx avx2")
	for _, want := range []string{"-O3", "-DNGX_VOD_MAX_TRACK_COUNT=256", "-mpopcnt", "-mavx2"} {
		if !contains(got, want) {
			t.Fatalf("missing %s in %s", want, got)
		}
	}
	plain := vodCompilerOpt("fp asimd")
	if contains(plain, "avx2") || contains(plain, "popcnt") {
		t.Fatalf("arm flags leaked into %s", plain)
	}
	if !contains(plain, "-O3") || !contains(plain, "-DNGX_VOD_MAX_TRACK_COUNT=256") {
		t.Fatalf("base flags missing: %s", plain)
	}
}

func TestPatchVodSource(t *testing.T) {
	src := "static void ngx_http_vod_exit_process();\nstatic void \nngx_http_vod_exit_process()\n{\n#if (VOD_HAVE_ICONV)\n\twebvtt_exit_process();\n#endif\n}\n"
	got, err := patchVodSource(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"static void ngx_http_vod_exit_process(ngx_cycle_t *cycle);",
		"ngx_http_vod_exit_process(ngx_cycle_t *cycle)",
		"(void)cycle;",
	} {
		if !contains(got, want) {
			t.Fatalf("missing %s\n%s", want, got)
		}
	}
	if contains(got, "ngx_http_vod_exit_process()") {
		t.Fatalf("old declaration remains\n%s", got)
	}
	again, err := patchVodSource(got)
	if err != nil || again != got {
		t.Fatalf("second pass changed source: %v", err)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && (s == sub || len(s) > 0 && indexOf(s, sub) >= 0))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

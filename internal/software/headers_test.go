package software

import (
	"strings"
	"testing"
)

func TestWithoutHeaderBuffers(t *testing.T) {
	in := "sendfile on;\nclient_header_buffer_size 1k;\nlarge_client_header_buffers 4 8k;\n# large_client_header_buffers 4 8k;\ngzip on;\n"
	got, changed := withoutHeaderBuffers(in)
	if !changed {
		t.Fatal("expected the buffer lines to be removed")
	}
	for _, line := range strings.Split(got, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "client_header_buffer_size") || strings.HasPrefix(trim, "large_client_header_buffers") {
			t.Fatalf("buffer lines remain:\n%s", got)
		}
	}
	if !stringsContains(got, "sendfile on;") || !stringsContains(got, "gzip on;") || !stringsContains(got, "# large_client_header_buffers 4 8k;") {
		t.Fatalf("other lines changed:\n%s", got)
	}
	again, changed := withoutHeaderBuffers(got)
	if changed || again != got {
		t.Fatal("second pass should leave the file alone")
	}
	if !stringsContains(headerBufferConf, "large_client_header_buffers 4 128k;") {
		t.Fatal("header buffer conf")
	}
}

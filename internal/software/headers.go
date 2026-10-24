package software

import "strings"

// headerBufferConf raises the buffer that must hold one whole request header.
// Nginx's built-in 8k answers 400 Request Header Or Cookie Too Large.
const headerBufferConf = `# Siroc request header buffers
# One buffer holds a whole Cookie header. The default 8k is too small.
client_header_buffer_size 4k;
large_client_header_buffers 4 128k;
`

// apacheHeaderConf is the same limit behind Nginx, so a large cookie is not
// refused by Apache after Nginx has accepted it.
const apacheHeaderConf = `# Siroc request header size
LimitRequestFieldSize 131072
`

func withoutHeaderBuffers(conf string) (string, bool) {
	lines := strings.Split(conf, "\n")
	out := make([]string, 0, len(lines))
	changed := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			out = append(out, line)
			continue
		}
		fields := strings.Fields(trim)
		key := strings.TrimSuffix(fields[0], ";")
		if key == "client_header_buffer_size" || key == "large_client_header_buffers" {
			changed = true
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n"), changed
}

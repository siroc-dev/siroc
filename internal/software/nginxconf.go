package software

import "strings"

var sirocLoadModules = []string{
	"load_module /usr/lib/nginx/modules/ndk_http_module.so;",
	"load_module /usr/lib/nginx/modules/ngx_http_lua_module.so;",
	"load_module /usr/lib/nginx/modules/ngx_http_vod_module.so;",
}

func nginxHasDirective(conf, line string) bool {
	for _, raw := range strings.Split(conf, "\n") {
		trim := strings.TrimSpace(raw)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		if trim == line {
			return true
		}
	}
	return false
}

// stripSirocLoadModules removes the dynamic module lines this panel adds.
// A package reinstall starts Nginx before those modules are rebuilt, and the
// Lua module refuses to start until resty.core is installed.
func stripSirocLoadModules(conf string) (string, bool) {
	var out []string
	changed := false
	for _, raw := range strings.Split(conf, "\n") {
		trim := strings.TrimSpace(raw)
		drop := false
		for _, line := range sirocLoadModules {
			if trim == line {
				drop = true
				break
			}
		}
		if drop {
			changed = true
			continue
		}
		out = append(out, raw)
	}
	return strings.Join(out, "\n"), changed
}

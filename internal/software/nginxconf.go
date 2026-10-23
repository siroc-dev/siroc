package software

import "strings"

var sirocLoadModules = []string{
	"load_module /usr/lib/nginx/modules/ndk_http_module.so;",
	"load_module /usr/lib/nginx/modules/ngx_http_lua_module.so;",
	"load_module /usr/lib/nginx/modules/ngx_http_vod_module.so;",
}

// NginxModuleStale reports a dynamic module built for a different Nginx
// than the one now installed, for example "version 1031007 instead of 1031008".
func NginxModuleStale(output string) bool {
	return strings.Contains(output, ".so") && strings.Contains(output, "version ") && strings.Contains(output, "instead of")
}

// NginxModuleHook is the apt snippet that runs after a package install.
// The binary is this agent. The command only rebuilds when the modules do not match.
func NginxModuleHook(bin string) string {
	bin = strings.TrimSpace(bin)
	if bin == "" {
		bin = "/usr/local/bin/siroc-agent"
	}
	return "// Added by siroc. An upgraded Nginx must not load modules built for the previous version.\n" +
		"DPkg::Post-Invoke { \"" + bin + " nginx-modules-hook || true\"; };\n"
}

func aptLockBusy(msg string) bool {
	s := strings.ToLower(msg)
	return strings.Contains(s, "could not get lock") || strings.Contains(s, "dpkg frontend lock") || strings.Contains(s, "resource temporarily unavailable")
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

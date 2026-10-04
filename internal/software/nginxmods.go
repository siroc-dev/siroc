//go:build linux

package software

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	nginxModDir   = "/usr/lib/nginx/modules"
	nginxModStamp = "/var/lib/siroc/nginx-modules.stamp"
	nginxBuildDir = "/var/lib/siroc/nginx-build"
	vodVersion    = "1.33"
	luaVersion    = "v0.10.26"
	ndkVersion    = "v0.3.3"
)

var nginxModMu sync.Mutex

// EnsureNginxModules configures the core thread pool and builds the Kaltura
// VOD module plus ngx_http_lua_module for the installed Nginx.
func EnsureNginxModules() error {
	nginxModMu.Lock()
	defer nginxModMu.Unlock()
	if _, err := exec.LookPath("nginx"); err != nil {
		return nil
	}
	changed, err := ensureThreadPool()
	if err != nil {
		return err
	}
	ver, err := installedNginxVersion()
	if err != nil {
		return err
	}
	stamp := ver + " vod=" + vodVersion + " lua=" + luaVersion + " ndk=" + ndkVersion + " cc=" + vodCompilerOpt(cpuFlags())
	if !nginxModulesReady(stamp) {
		if err := buildNginxModules(ver); err != nil {
			return err
		}
		changed = true
	}
	loaded, err := ensureLoadModules()
	if err != nil {
		return err
	}
	vodConf, err := ensureVodRuntime()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(nginxModStamp), 0750); err != nil {
		return err
	}
	if err := os.WriteFile(nginxModStamp, []byte(stamp+"\n"), 0644); err != nil {
		return err
	}
	if changed || loaded || vodConf {
		if exec.Command("systemctl", "is-active", "--quiet", "nginx").Run() == nil {
			if out, err := exec.Command("systemctl", "restart", "nginx").CombinedOutput(); err != nil {
				return fmt.Errorf("restart nginx: %s: %w", strings.TrimSpace(string(out)), err)
			}
		}
	}
	return nil
}

func nginxModulesReady(stamp string) bool {
	b, err := os.ReadFile(nginxModStamp)
	if err != nil || strings.TrimSpace(string(b)) != stamp {
		return false
	}
	for _, name := range []string{"ndk_http_module.so", "ngx_http_lua_module.so", "ngx_http_vod_module.so"} {
		if _, err := os.Stat(filepath.Join(nginxModDir, name)); err != nil {
			return false
		}
	}
	return true
}

func installedNginxVersion() (string, error) {
	out, err := exec.Command("nginx", "-v").CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil && s == "" {
		return "", err
	}
	i := strings.LastIndex(s, "/")
	if i < 0 || i == len(s)-1 {
		return "", fmt.Errorf("nginx version: %s", s)
	}
	ver := strings.TrimSpace(s[i+1:])
	if ver == "" || strings.ContainsAny(ver, " \n/") {
		return "", fmt.Errorf("nginx version: %s", s)
	}
	return ver, nil
}

func ensureThreadPool() (bool, error) {
	path := "/etc/nginx/nginx.conf"
	b, err := os.ReadFile(path)
	if err != nil {
		return false, nil
	}
	if strings.Contains(string(b), "thread_pool ") {
		return false, nil
	}
	line := "thread_pool default threads=32 max_queue=65536;\n"
	conf := string(b)
	if i := strings.Index(conf, "events"); i >= 0 {
		conf = conf[:i] + line + conf[i:]
	} else {
		conf = line + conf
	}
	if err := os.WriteFile(path, []byte(conf), 0644); err != nil {
		return false, err
	}
	if err := nginxConfigTest(); err != nil {
		_ = os.WriteFile(path, b, 0644)
		return false, err
	}
	return true, nil
}

func ensureLoadModules() (bool, error) {
	path := "/etc/nginx/nginx.conf"
	b, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read nginx.conf: %w", err)
	}
	lines := []string{
		"load_module /usr/lib/nginx/modules/ndk_http_module.so;",
		"load_module /usr/lib/nginx/modules/ngx_http_lua_module.so;",
		"load_module /usr/lib/nginx/modules/ngx_http_vod_module.so;",
	}
	conf := string(b)
	var missing []string
	for _, line := range lines {
		if !strings.Contains(conf, line) {
			missing = append(missing, line)
		}
	}
	if len(missing) == 0 {
		return false, nil
	}
	conf = strings.Join(missing, "\n") + "\n" + conf
	if err := os.WriteFile(path, []byte(conf), 0644); err != nil {
		return false, err
	}
	if err := nginxConfigTest(); err != nil {
		_ = os.WriteFile(path, b, 0644)
		return false, err
	}
	return true, nil
}

func cpuFlags() string {
	b, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	var parts []string
	for _, line := range strings.Split(string(b), "\n") {
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "flags") || strings.HasPrefix(lower, "features") {
			if i := strings.Index(line, ":"); i >= 0 {
				parts = append(parts, line[i+1:])
			}
		}
	}
	return strings.Join(parts, " ")
}

func nginxBuiltWith(opt string) bool {
	out, _ := exec.Command("nginx", "-V").CombinedOutput()
	return strings.Contains(string(out), opt)
}

func ensureVodRuntime() (bool, error) {
	path := "/etc/nginx/conf.d/siroc-vod.conf"
	if _, err := os.Stat(filepath.Join(nginxModDir, "ngx_http_vod_module.so")); err != nil {
		if _, statErr := os.Stat(path); statErr == nil {
			_ = os.Remove(path)
			return true, nil
		}
		return false, nil
	}
	var lines []string
	if nginxBuiltWith("--with-file-aio") {
		lines = append(lines, "aio on;")
	}
	if nginxBuiltWith("--with-threads") {
		lines = append(lines, "vod_open_file_thread_pool default;")
	}
	if len(lines) == 0 {
		return false, nil
	}
	body := "# Siroc nginx-vod-module\n" + strings.Join(lines, "\n") + "\n"
	prev, _ := os.ReadFile(path)
	if string(prev) == body {
		return false, nil
	}
	if err := os.MkdirAll("/etc/nginx/conf.d", 0755); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		return false, err
	}
	if err := nginxConfigTest(); err != nil {
		if len(prev) == 0 {
			_ = os.Remove(path)
		} else {
			_ = os.WriteFile(path, prev, 0644)
		}
		noteInstall("vod runtime config skipped: " + err.Error())
		return false, nil
	}
	return true, nil
}

func nginxConfigTest() error {
	out, err := exec.Command("nginx", "-t").CombinedOutput()
	if err != nil {
		return fmt.Errorf("nginx -t: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func buildNginxModules(ver string) error {
	if err := aptInstall("build-essential", "libpcre2-dev", "zlib1g-dev", "libssl-dev", "wget", "ca-certificates", "curl", "libluajit-5.1-dev"); err != nil {
		return err
	}
	luaEnv, err := luaJITEnv()
	if err != nil {
		return err
	}
	_ = os.RemoveAll(nginxBuildDir)
	if err := os.MkdirAll(nginxBuildDir, 0755); err != nil {
		return err
	}
	defer os.RemoveAll(nginxBuildDir)
	sources := []struct{ url, dir string }{
		{"https://nginx.org/download/nginx-" + ver + ".tar.gz", "nginx"},
		{"https://github.com/kaltura/nginx-vod-module/archive/refs/tags/" + vodVersion + ".tar.gz", "vod"},
		{"https://github.com/vision5/ngx_devel_kit/archive/refs/tags/" + ndkVersion + ".tar.gz", "ndk"},
		{"https://github.com/openresty/lua-nginx-module/archive/refs/tags/" + luaVersion + ".tar.gz", "lua"},
	}
	for _, src := range sources {
		if err := fetchTar(src.url, filepath.Join(nginxBuildDir, src.dir)); err != nil {
			return err
		}
	}
	vodSrc := filepath.Join(nginxBuildDir, "vod", "ngx_http_vod_module.c")
	body, err := os.ReadFile(vodSrc)
	if err != nil {
		return err
	}
	patched, err := patchVodSource(string(body))
	if err != nil {
		return err
	}
	if err := os.WriteFile(vodSrc, []byte(patched), 0644); err != nil {
		return err
	}
	dfxp := filepath.Join(nginxBuildDir, "vod", "subtitle", "dfxp_format.c")
	dfxpBody, err := os.ReadFile(dfxp)
	if err != nil {
		return err
	}
	dfxpPatched, err := patchVodDFXP(string(dfxpBody))
	if err != nil {
		return err
	}
	if err := os.WriteFile(dfxp, []byte(dfxpPatched), 0644); err != nil {
		return err
	}
	cc := vodCompilerOpt(cpuFlags())
	noteInstall("vod cc-opt: " + cc)
	cmd := exec.Command("./configure", "--with-compat",
		"--with-file-aio",
		"--with-threads",
		"--with-cc-opt="+cc,
		"--add-dynamic-module="+filepath.Join(nginxBuildDir, "ndk"),
		"--add-dynamic-module="+filepath.Join(nginxBuildDir, "lua"),
		"--add-dynamic-module="+filepath.Join(nginxBuildDir, "vod"),
	)
	cmd.Dir = filepath.Join(nginxBuildDir, "nginx")
	cmd.Env = append(os.Environ(), luaEnv...)
	if out, err := combinedTimeout(cmd, 5*time.Minute); err != nil {
		return fmt.Errorf("nginx configure: %s: %w", tail(out), err)
	}
	jobs := runtime.NumCPU()
	if jobs < 1 {
		jobs = 1
	}
	makeCmd := exec.Command("make", "-j"+strconv.Itoa(jobs), "modules")
	makeCmd.Dir = cmd.Dir
	makeCmd.Env = cmd.Env
	if out, err := combinedTimeout(makeCmd, 20*time.Minute); err != nil {
		return fmt.Errorf("nginx modules: %s: %w", tail(out), err)
	}
	if err := os.MkdirAll(nginxModDir, 0755); err != nil {
		return err
	}
	objs := filepath.Join(nginxBuildDir, "nginx", "objs")
	for _, name := range []string{"ndk_http_module.so", "ngx_http_lua_module.so", "ngx_http_vod_module.so"} {
		src := filepath.Join(objs, name)
		if _, err := os.Stat(src); err != nil {
			return fmt.Errorf("missing %s", name)
		}
		if err := copyFile(src, filepath.Join(nginxModDir, name)); err != nil {
			return err
		}
	}
	return nil
}

func fetchTar(url, dest string) error {
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}
	arc := dest + ".tar.gz"
	cmd := exec.Command("curl", "-fsSL", "-o", arc, url)
	if out, err := combinedTimeout(cmd, 3*time.Minute); err != nil {
		return fmt.Errorf("download %s: %s: %w", url, tail(out), err)
	}
	extract := exec.Command("tar", "-xzf", arc, "-C", dest, "--strip-components=1")
	if out, err := extract.CombinedOutput(); err != nil {
		return fmt.Errorf("extract %s: %s: %w", url, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func luaJITEnv() ([]string, error) {
	var inc string
	for _, p := range []string{"/usr/include/luajit-2.1", "/usr/include/luajit-2.0", "/usr/local/include/luajit-2.1"} {
		if _, err := os.Stat(filepath.Join(p, "luajit.h")); err == nil {
			inc = p
			break
		}
	}
	if inc == "" {
		return nil, fmt.Errorf("LuaJIT headers not found")
	}
	var lib string
	for _, p := range []string{"/usr/lib/x86_64-linux-gnu", "/usr/lib/aarch64-linux-gnu", "/usr/lib64", "/usr/lib", "/usr/local/lib"} {
		if _, err := os.Stat(filepath.Join(p, "libluajit-5.1.so")); err == nil {
			lib = p
			break
		}
	}
	if lib == "" {
		return nil, fmt.Errorf("libluajit-5.1.so not found")
	}
	return []string{"LUAJIT_INC=" + inc, "LUAJIT_LIB=" + lib}, nil
}

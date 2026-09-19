//go:build linux

package hosting

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/software"
	"github.com/siroc-dev/siroc/internal/validate"
)

func (m *Manager) ApplyUserPHP(req rpc.PHPUserApplyReq) error {
	if err := validate.LinuxUser(req.Username); err != nil {
		return err
	}
	settings, err := normalizeFPM(req.Settings)
	if err != nil {
		return err
	}
	settings.OpenBasedir = true
	versions := map[string]struct{}{}
	for ver := range settings.Extensions {
		if err := validate.PHPVersion(ver); err != nil {
			return err
		}
		versions[ver] = struct{}{}
	}
	for _, st := range req.Sites {
		if err := validate.PHPVersion(st.PHPVersion); err != nil {
			return err
		}
		versions[st.PHPVersion] = struct{}{}
	}
	for ver := range versions {
		if err := m.writeUserPHP(req.Username, ver, settings); err != nil {
			return err
		}
	}
	for _, st := range req.Sites {
		st.Username = req.Username
		st.FPM = &settings
		if err := m.Write(st); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) writeUserPHP(username, ver string, settings rpc.PHPFPMSettings) error {
	home := filepath.Join(m.HomeRoot, username)
	dir := filepath.Join(home, ".php", ver)
	confd := filepath.Join(dir, "conf.d")
	if err := os.RemoveAll(confd); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(confd, 0750); err != nil {
		return err
	}
	srcIni := filepath.Join("/etc/php", ver, "fpm", "php.ini")
	b, err := os.ReadFile(srcIni)
	if err != nil {
		return fmt.Errorf("php.ini for %s: %w", ver, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "php.ini"), b, 0640); err != nil {
		return err
	}
	selected := settings.Extensions[ver]
	if selected == nil {
		selected = software.EnabledPHPExtNames(ver)
	}
	selected = append(software.CorePHPExtNames(ver), selected...)
	for i, src := range software.PHPExtIniFiles(ver, selected) {
		data, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		name := filepath.Base(src)
		prio := strconv.Itoa(10 + (i % 80))
		if err := os.WriteFile(filepath.Join(confd, prio+"-"+name), data, 0640); err != nil {
			return err
		}
	}
	_ = exec.Command("chown", "-R", username+":"+username, filepath.Join(home, ".php")).Run()
	return nil
}

func (m *Manager) LockOpenBasedir() {
	files, err := filepath.Glob("/etc/php/*/fpm/pool.d/*.conf")
	if err != nil {
		return
	}
	reload := map[string]struct{}{}
	for _, path := range files {
		base := filepath.Base(path)
		if base == "www.conf" || strings.HasPrefix(base, "www.") {
			continue
		}
		changed, ver := m.lockPoolFile(path)
		if changed && ver != "" {
			reload[ver] = struct{}{}
		}
	}
	for ver := range reload {
		_ = exec.Command("systemctl", "reload", "php"+ver+"-fpm").Run()
	}
}

func (m *Manager) lockPoolFile(path string) (bool, string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return false, ""
	}
	user := poolUser(string(b))
	if user == "" || user == "www-data" || validate.LinuxUser(user) != nil {
		return false, ""
	}
	home, tmp, err := validate.HomeJail(m.HomeRoot, user)
	if err != nil {
		return false, ""
	}
	_ = os.MkdirAll(tmp, 0750)
	_ = exec.Command("chown", user+":"+user, tmp).Run()
	next := setPoolValue(string(b), "open_basedir", home+string(os.PathSeparator))
	next = setPoolValue(next, "session.save_path", tmp)
	next = setPoolValue(next, "upload_tmp_dir", tmp)
	next = setPoolValue(next, "sys_temp_dir", tmp)
	if next == string(b) {
		return false, phpVerFromPoolPath(path)
	}
	if err := os.WriteFile(path, []byte(next), 0644); err != nil {
		return false, ""
	}
	return true, phpVerFromPoolPath(path)
}

func poolUser(content string) string {
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, "user") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) != "user" {
			continue
		}
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func setPoolValue(content, key, value string) string {
	prefix := "php_admin_value[" + key + "]"
	line := prefix + " = " + value
	var b strings.Builder
	found := false
	for _, raw := range strings.SplitAfter(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(raw), prefix) {
			b.WriteString(line + "\n")
			found = true
			continue
		}
		b.WriteString(raw)
	}
	out := b.String()
	if !found {
		if !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		out += line + "\n"
	}
	return out
}

func phpVerFromPoolPath(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for i, p := range parts {
		if p == "php" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func normalizeFPM(in rpc.PHPFPMSettings) (rpc.PHPFPMSettings, error) {
	s := rpc.MergePHPFPM(in)
	if err := validate.PHPFPM(s.PM, s.MaxChildren, s.StartServers, s.MinSpare, s.MaxSpare, s.MaxRequests, s.MaxExecutionTime, s.MaxInputTime); err != nil {
		return s, err
	}
	if err := validate.PHPSize(s.MemoryLimit, "memory_limit"); err != nil {
		return s, err
	}
	if err := validate.PHPSize(s.PostMaxSize, "post_max_size"); err != nil {
		return s, err
	}
	if err := validate.PHPSize(s.UploadMaxFilesize, "upload_max_filesize"); err != nil {
		return s, err
	}
	if err := validate.PHPTimezone(s.Timezone); err != nil {
		return s, err
	}
	if err := validate.PHPIdleTimeout(s.IdleTimeout); err != nil {
		return s, err
	}
	if err := validate.PHPDisableFunctions(s.DisableFunctions); err != nil {
		return s, err
	}
	exts := map[string][]string{}
	for ver, names := range s.Extensions {
		if err := validate.PHPVersion(ver); err != nil {
			return s, err
		}
		var clean []string
		seen := map[string]struct{}{}
		for _, n := range names {
			n = strings.ToLower(strings.TrimSpace(n))
			if n == "" {
				continue
			}
			if err := validate.PHPExtName(n); err != nil {
				return s, err
			}
			if _, ok := seen[n]; ok {
				continue
			}
			seen[n] = struct{}{}
			clean = append(clean, n)
		}
		if clean == nil {
			clean = []string{}
		}
		exts[ver] = clean
	}
	s.Extensions = exts
	return s, nil
}

func fillSiteFPM(data *siteData, homeRoot string, req rpc.SiteWriteReq) error {
	settings := rpc.DefaultPHPFPM()
	custom := req.FPM != nil
	if custom {
		var err error
		settings, err = normalizeFPM(*req.FPM)
		if err != nil {
			return err
		}
	}
	data.PM = settings.PM
	data.MaxChildren = settings.MaxChildren
	data.StartServers = settings.StartServers
	data.MinSpare = settings.MinSpare
	data.MaxSpare = settings.MaxSpare
	data.IdleTimeout = settings.IdleTimeout
	data.MaxRequests = settings.MaxRequests
	data.MemoryLimit = settings.MemoryLimit
	data.MaxExecutionTime = settings.MaxExecutionTime
	data.MaxInputTime = settings.MaxInputTime
	data.PostMaxSize = settings.PostMaxSize
	data.UploadMaxFilesize = settings.UploadMaxFilesize
	data.Timezone = settings.Timezone
	data.DisableFunctions = settings.DisableFunctions
	data.DisplayErrors = "off"
	if settings.DisplayErrors {
		data.DisplayErrors = "on"
	}
	home, tmp, err := validate.HomeJail(homeRoot, req.Username)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(tmp, 0750); err != nil {
		return err
	}
	data.OpenBasedir = home + string(os.PathSeparator)
	data.UserTmp = tmp
	if custom {
		data.PHPIniDir = filepath.Join(home, ".php", req.PHPVersion)
	}
	return nil
}

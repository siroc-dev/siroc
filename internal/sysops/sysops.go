//go:build linux

package sysops

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/software"
)

func Status() (*rpc.SysopsStatus, error) {
	st := &rpc.SysopsStatus{
		Hostname: hostname(),
		Time:     time.Now().In(localLoc()).Format(time.RFC3339),
		Timezone: timezone(),
		NTP:      ntpOn(),
		DNS:      nameservers(),
		Search:   searchDomain(),
		Swap:     swapInfo(),
		IPs:      addrs(),
		Routes:   routes(),
		Ifaces:   ifaces(),
		Mounts:   mounts(),
		Fstab:    fstab(),
		Fail2ban: fail2banStatus(),
		Threats:  threats(),
		FFmpeg:   ffmpegStatus(),
		Memcached: memcachedStatus(),
		Quota:     quotaReady(),
	}
	st.Timezones = listTimezones()
	return st, nil
}

func listTimezones() []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(z string) {
		z = strings.TrimSpace(z)
		if z == "" || strings.HasPrefix(z, "#") {
			return
		}
		if _, ok := seen[z]; ok {
			return
		}
		seen[z] = struct{}{}
		out = append(out, z)
	}
	add("UTC")
	add("Etc/UTC")
	add("Asia/Bangkok")
	for _, tab := range []string{"/usr/share/zoneinfo/zone1970.tab", "/usr/share/zoneinfo/zone.tab"} {
		f, err := os.Open(tab)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				add(fields[2])
			}
		}
		f.Close()
		if len(out) > 8 {
			break
		}
	}
	if len(out) <= 8 {
		_ = filepath.Walk("/usr/share/zoneinfo", func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			rel, err := filepath.Rel("/usr/share/zoneinfo", path)
			if err != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			if strings.HasPrefix(rel, "posix/") || strings.HasPrefix(rel, "right/") {
				return nil
			}
			base := filepath.Base(rel)
			switch base {
			case "zone.tab", "zone1970.tab", "iso3166.tab", "leapseconds", "tzdata.zi", "leap-seconds.list":
				return nil
			}
			if strings.HasPrefix(base, ".") {
				return nil
			}
			add(rel)
			return nil
		})
	}
	sort.Strings(out)
	return out
}

func Apply(req rpc.SysopsReq) (*rpc.SysopsStatus, error) {
	var err error
	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case "", "status":
	case "dns":
		err = setDNS(req.Nameservers, req.SearchDomain)
	case "timezone":
		err = setTimezone(req.Timezone, req.NTP)
	case "swap":
		err = setSwap(req.SwapMB)
	case "ip-add":
		err = addIP(req.Interface, req.Address)
	case "ip-del":
		err = delIP(req.Interface, req.Address)
	case "route":
		err = setGateway(req.Gateway, req.Interface)
	case "mount":
		err = doMount(req)
	case "umount":
		err = doUmount(req.MountPoint)
	case "fstab-add":
		err = addFstab(req)
	case "fstab-del":
		err = delFstab(req.MountPoint)
	case "unban":
		err = unban(req.Jail, req.IP)
	case "ffmpeg":
		out, e := ffmpegConvert(req.Src, req.Dest, req.Extra)
		st, _ := Status()
		if st.FFmpeg == nil {
			st.FFmpeg = &rpc.FFmpegStatus{}
		}
		st.FFmpeg.Output = out
		if e != nil {
			st.Message = e.Error()
			return st, e
		}
		return st, nil
	case "memcached":
		err = setMemcached(req.MemoryMB, req.Listen, req.Port)
	default:
		return nil, fmt.Errorf("unknown action %q", req.Action)
	}
	st, e2 := Status()
	if err != nil {
		if st != nil {
			st.Message = err.Error()
		}
		return st, err
	}
	return st, e2
}

func Disk() (*rpc.DiskAnalysis, error) {
	out := &rpc.DiskAnalysis{}
	for _, m := range mounts() {
		if m.FSType == "tmpfs" || m.FSType == "devtmpfs" || m.FSType == "overlay" || m.FSType == "squashfs" {
			continue
		}
		out.Filesystems = append(out.Filesystems, m)
	}
	for _, root := range []string{"/", "/home", "/var", "/opt"} {
		if st, err := os.Stat(root); err != nil || !st.IsDir() {
			continue
		}
		cmd := exec.Command("du", "-x", "-B1", "--max-depth=1", root)
		cmd.Env = os.Environ()
		done := make(chan []byte, 1)
		go func() {
			b, _ := cmd.CombinedOutput()
			done <- b
		}()
		select {
		case b := <-done:
			for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
				parts := strings.Fields(line)
				if len(parts) < 2 {
					continue
				}
				n, _ := strconv.ParseInt(parts[0], 10, 64)
				out.Trees = append(out.Trees, rpc.DiskNode{Path: parts[1], Size: n})
			}
		case <-time.After(12 * time.Second):
			_ = cmd.Process.Kill()
			out.Message = "directory sizes timed out on " + root
		}
	}
	b, _ := exec.Command("bash", "-lc", `find /home /var /opt /usr -xdev -type f -size +20M -printf '%s %p\n' 2>/dev/null | sort -nr | head -n 30`).CombinedOutput()
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), " ", 2)
		if len(parts) != 2 {
			continue
		}
		n, _ := strconv.ParseInt(parts[0], 10, 64)
		out.Largest = append(out.Largest, rpc.DiskNode{Path: parts[1], Size: n})
	}
	return out, nil
}

func SetQuota(username string, limitMB int64) (*rpc.QuotaInfo, error) {
	info, _ := GetQuota(username)
	if err := ensureQuota(); err != nil {
		if info != nil {
			info.Message = err.Error()
		}
		return info, err
	}
	kb := limitMB * 1024
	if limitMB <= 0 {
		kb = 0
	}
	fs := quotaFS()
	cmd := exec.Command("setquota", "-u", username, strconv.FormatInt(kb, 10), strconv.FormatInt(kb, 10), "0", "0", fs)
	if out, err := cmd.CombinedOutput(); err != nil {
		return info, fmt.Errorf("setquota: %s", strings.TrimSpace(string(out)))
	}
	return GetQuota(username)
}

func GetQuota(username string) (*rpc.QuotaInfo, error) {
	info := &rpc.QuotaInfo{Username: username}
	home := filepath.Join("/home", username)
	if b, err := exec.Command("du", "-sb", home).CombinedOutput(); err == nil {
		fields := strings.Fields(string(b))
		if len(fields) > 0 {
			info.UsedBytes, _ = strconv.ParseInt(fields[0], 10, 64)
		}
	}
	if !quotaReady() {
		info.Message = "kernel quotas are not enabled on this filesystem (common in Docker)"
		return info, nil
	}
	b, err := exec.Command("quota", "-u", username, "-w", "-p").CombinedOutput()
	if err != nil && len(b) == 0 {
		info.Message = strings.TrimSpace(string(b))
		return info, nil
	}
	info.Enabled = true
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		if !strings.HasPrefix(fields[0], "/") && !strings.Contains(fields[0], "/") {
			continue
		}
		used, _ := strconv.ParseInt(strings.TrimRight(fields[1], "*"), 10, 64)
		lim, _ := strconv.ParseInt(fields[3], 10, 64)
		if used > 0 {
			info.UsedBytes = used * 1024
		}
		info.LimitBytes = lim * 1024
		if lim > 0 {
			info.LimitMB = lim / 1024
		}
	}
	return info, nil
}

func hostname() string {
	h, _ := os.Hostname()
	return h
}

func localLoc() *time.Location {
	if z := timezone(); z != "" {
		if loc, err := time.LoadLocation(z); err == nil {
			return loc
		}
	}
	return time.Local
}

func timezone() string {
	if b, err := os.ReadFile("/etc/timezone"); err == nil {
		if z := strings.TrimSpace(string(b)); z != "" {
			return z
		}
	}
	if link, err := os.Readlink("/etc/localtime"); err == nil {
		if i := strings.Index(link, "zoneinfo/"); i >= 0 {
			return link[i+len("zoneinfo/"):]
		}
	}
	return ""
}

func ntpOn() bool {
	b, err := exec.Command("timedatectl", "show", "-p", "NTP", "--value").CombinedOutput()
	if err == nil && strings.TrimSpace(string(b)) == "yes" {
		return true
	}
	for _, unit := range []string{"systemd-timesyncd", "chrony", "chronyd", "ntp", "ntpd"} {
		if exec.Command("systemctl", "is-active", "--quiet", unit).Run() == nil {
			return true
		}
	}
	return false
}

func nameservers() []string {
	f, err := os.Open("/etc/resolv.conf")
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "nameserver ") {
			out = append(out, strings.TrimSpace(strings.TrimPrefix(line, "nameserver ")))
		}
	}
	return out
}

func searchDomain() string {
	f, err := os.Open("/etc/resolv.conf")
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "search ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "search "))
		}
	}
	return ""
}

func setDNS(ns []string, search string) error {
	var b strings.Builder
	b.WriteString("# managed by siroc\n")
	for _, n := range ns {
		n = strings.TrimSpace(n)
		if net.ParseIP(n) == nil {
			return fmt.Errorf("invalid nameserver %q", n)
		}
		b.WriteString("nameserver " + n + "\n")
	}
	if s := strings.TrimSpace(search); s != "" {
		b.WriteString("search " + s + "\n")
	}
	_ = os.Rename("/etc/resolv.conf", "/etc/resolv.conf.cpbak")
	return os.WriteFile("/etc/resolv.conf", []byte(b.String()), 0644)
}

func setTimezone(tz string, ntp *bool) error {
	if tz != "" {
		if err := applyTimezone(tz); err != nil {
			return err
		}
	}
	if ntp != nil {
		setNTP(*ntp)
	}
	return nil
}

func zoneinfoPath(tz string) (string, error) {
	tz = strings.TrimSpace(tz)
	if tz == "" || strings.ContainsAny(tz, " \n;\x00") || strings.Contains(tz, "..") {
		return "", fmt.Errorf("invalid timezone")
	}
	src := filepath.Clean(filepath.Join("/usr/share/zoneinfo", tz))
	if src != "/usr/share/zoneinfo" && !strings.HasPrefix(src, "/usr/share/zoneinfo"+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid timezone")
	}
	st, err := os.Stat(src)
	if err != nil || st.IsDir() {
		return "", fmt.Errorf("unknown timezone %q", tz)
	}
	return src, nil
}

func applyTimezone(tz string) error {
	src, err := zoneinfoPath(tz)
	if err != nil {
		return err
	}
	if exec.Command("timedatectl", "set-timezone", tz).Run() == nil {
		return nil
	}
	if err := os.WriteFile("/etc/timezone", []byte(tz+"\n"), 0644); err != nil {
		return fmt.Errorf("timezone: %w", err)
	}
	_ = os.Remove("/etc/localtime")
	if err := os.Symlink(src, "/etc/localtime"); err != nil {
		b, rerr := os.ReadFile(src)
		if rerr != nil {
			return fmt.Errorf("timezone: %w", err)
		}
		if werr := os.WriteFile("/etc/localtime", b, 0644); werr != nil {
			return fmt.Errorf("timezone: %w", werr)
		}
	}
	return nil
}

func setNTP(on bool) {
	v := "0"
	if on {
		v = "1"
	}
	if exec.Command("timedatectl", "set-ntp", v).Run() == nil {
		return
	}
	units := []string{"systemd-timesyncd", "chrony", "chronyd", "ntp", "ntpd"}
	for _, unit := range units {
		if on {
			if exec.Command("systemctl", "enable", "--now", unit).Run() == nil {
				return
			}
			continue
		}
		_ = exec.Command("systemctl", "disable", "--now", unit).Run()
	}
}

func swapInfo() *rpc.SwapInfo {
	info := &rpc.SwapInfo{}
	b, _ := exec.Command("swapon", "--show=NAME,SIZE,USED", "--bytes", "--noheadings").CombinedOutput()
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		sz, _ := strconv.ParseInt(fields[1], 10, 64)
		used, _ := strconv.ParseInt(fields[2], 10, 64)
		info.TotalMB += int(sz / (1024 * 1024))
		info.UsedMB += int(used / (1024 * 1024))
		if info.File == "" {
			info.File = fields[0]
		}
	}
	return info
}

func setSwap(mb int) error {
	const path = "/swapfile"
	if mb <= 0 {
		_ = exec.Command("swapoff", path).Run()
		_ = os.Remove(path)
		return delFstab(path)
	}
	if mb < 64 || mb > 32768 {
		return fmt.Errorf("swap size must be 64–32768 MB")
	}
	_ = exec.Command("swapoff", path).Run()
	_ = os.Remove(path)
	cmd := exec.Command("fallocate", "-l", fmt.Sprintf("%dM", mb), path)
	if out, err := cmd.CombinedOutput(); err != nil {
		cmd = exec.Command("dd", "if=/dev/zero", "of="+path, "bs=1M", fmt.Sprintf("count=%d", mb), "status=none")
		if out2, err2 := cmd.CombinedOutput(); err2 != nil {
			return fmt.Errorf("create swap: %s %s", strings.TrimSpace(string(out)), strings.TrimSpace(string(out2)))
		}
	}
	_ = os.Chmod(path, 0600)
	if out, err := exec.Command("mkswap", path).CombinedOutput(); err != nil {
		return fmt.Errorf("mkswap: %s", strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("swapon", path).CombinedOutput(); err != nil {
		return fmt.Errorf("swapon: %s", strings.TrimSpace(string(out)))
	}
	return addFstab(rpc.SysopsReq{Device: path, MountPoint: "none", FSType: "swap", Options: "sw"})
}

func ifaces() []string {
	fis, _ := os.ReadDir("/sys/class/net")
	var out []string
	for _, f := range fis {
		if f.Name() == "lo" {
			continue
		}
		out = append(out, f.Name())
	}
	return out
}

func addrs() []rpc.NetAddr {
	ifaces, _ := net.Interfaces()
	var out []rpc.NetAddr
	for _, ifi := range ifaces {
		as, _ := ifi.Addrs()
		for _, a := range as {
			fam := "inet"
			if strings.Contains(a.String(), ":") && !strings.Contains(a.String(), ".") {
				fam = "inet6"
			}
			out = append(out, rpc.NetAddr{Iface: ifi.Name, Address: a.String(), Family: fam})
		}
	}
	return out
}

func routes() []string {
	b, _ := exec.Command("ip", "-4", "route").CombinedOutput()
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

func addIP(iface, addr string) error {
	if iface == "" || addr == "" {
		return fmt.Errorf("interface and CIDR address required")
	}
	if _, _, err := net.ParseCIDR(addr); err != nil {
		return fmt.Errorf("address must be CIDR, e.g. 192.168.1.10/24")
	}
	if out, err := exec.Command("ip", "addr", "add", addr, "dev", iface).CombinedOutput(); err != nil {
		return fmt.Errorf("ip add: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func delIP(iface, addr string) error {
	if iface == "" || addr == "" {
		return fmt.Errorf("interface and address required")
	}
	if out, err := exec.Command("ip", "addr", "del", addr, "dev", iface).CombinedOutput(); err != nil {
		return fmt.Errorf("ip del: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func setGateway(gw, iface string) error {
	if net.ParseIP(gw) == nil {
		return fmt.Errorf("invalid gateway")
	}
	args := []string{"route", "replace", "default", "via", gw}
	if iface != "" {
		args = append(args, "dev", iface)
	}
	if out, err := exec.Command("ip", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("route: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func mounts() []rpc.MountInfo {
	b, _ := exec.Command("df", "-Th", "--exclude-type=tmpfs", "--exclude-type=devtmpfs", "--exclude-type=squashfs").CombinedOutput()
	var out []rpc.MountInfo
	for i, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if i == 0 {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 7 {
			continue
		}
		out = append(out, rpc.MountInfo{
			Device: f[0], FSType: f[1], Size: f[2], Used: f[3], Avail: f[4], UsePct: f[5], MountPoint: f[6],
		})
	}
	return out
}

func fstab() []rpc.FstabEntry {
	b, err := os.ReadFile("/etc/fstab")
	if err != nil {
		return nil
	}
	var out []rpc.FstabEntry
	for _, line := range strings.Split(string(b), "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		f := strings.Fields(trim)
		e := rpc.FstabEntry{Raw: trim}
		if len(f) >= 4 {
			e.Device, e.MountPoint, e.FSType, e.Options = f[0], f[1], f[2], f[3]
		}
		if len(f) >= 6 {
			e.Dump, e.Pass = f[4], f[5]
		}
		out = append(out, e)
	}
	return out
}

func doMount(req rpc.SysopsReq) error {
	if req.Device == "" || req.MountPoint == "" {
		return fmt.Errorf("device and mount point required")
	}
	if err := os.MkdirAll(req.MountPoint, 0755); err != nil {
		return err
	}
	args := []string{req.Device, req.MountPoint}
	if req.FSType != "" {
		args = append([]string{"-t", req.FSType}, args...)
	}
	if req.Options != "" {
		args = append([]string{"-o", req.Options}, args...)
	}
	if out, err := exec.Command("mount", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("mount: %s", strings.TrimSpace(string(out)))
	}
	if req.Persist {
		return addFstab(req)
	}
	return nil
}

func doUmount(mp string) error {
	if mp == "" || mp == "/" {
		return fmt.Errorf("invalid mount point")
	}
	if out, err := exec.Command("umount", mp).CombinedOutput(); err != nil {
		return fmt.Errorf("umount: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func addFstab(req rpc.SysopsReq) error {
	if req.Device == "" || req.MountPoint == "" {
		return fmt.Errorf("device and mount point required")
	}
	fs := req.FSType
	if fs == "" {
		fs = "auto"
	}
	opt := req.Options
	if opt == "" {
		if fs == "swap" {
			opt = "sw"
		} else {
			opt = "defaults"
		}
	}
	line := fmt.Sprintf("%s %s %s %s 0 0\n", req.Device, req.MountPoint, fs, opt)
	for _, e := range fstab() {
		if e.MountPoint == req.MountPoint || (fs == "swap" && e.Device == req.Device) {
			return nil
		}
	}
	f, err := os.OpenFile("/etc/fstab", os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line)
	return err
}

func delFstab(mp string) error {
	if mp == "" {
		return fmt.Errorf("mount point required")
	}
	b, err := os.ReadFile("/etc/fstab")
	if err != nil {
		return err
	}
	var keep []string
	for _, line := range strings.Split(string(b), "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			keep = append(keep, line)
			continue
		}
		f := strings.Fields(trim)
		if len(f) >= 2 && (f[1] == mp || f[0] == mp) {
			continue
		}
		keep = append(keep, line)
	}
	return os.WriteFile("/etc/fstab", []byte(strings.Join(keep, "\n")+"\n"), 0644)
}

func fail2banStatus() *rpc.Fail2banStatus {
	st := &rpc.Fail2banStatus{}
	if _, err := exec.LookPath("fail2ban-client"); err != nil {
		return st
	}
	st.Installed = true
	st.Active = exec.Command("systemctl", "is-active", "--quiet", "fail2ban").Run() == nil
	b, err := exec.Command("fail2ban-client", "status").CombinedOutput()
	if err != nil {
		st.Message = strings.TrimSpace(string(b))
		return st
	}
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.Contains(line, "Jail list:") {
			continue
		}
		part := strings.TrimSpace(line[strings.Index(line, "Jail list:")+len("Jail list:"):])
		for _, name := range strings.Split(part, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			st.Jails = append(st.Jails, jailInfo(name))
		}
	}
	return st
}

func jailInfo(name string) rpc.Fail2banJail {
	j := rpc.Fail2banJail{Name: name}
	b, err := exec.Command("fail2ban-client", "status", name).CombinedOutput()
	if err != nil {
		return j
	}
	for _, line := range strings.Split(string(b), "\n") {
		low := strings.ToLower(line)
		if strings.Contains(low, "currently banned:") {
			j.Total, _ = strconv.Atoi(strings.TrimSpace(line[strings.LastIndex(line, ":")+1:]))
		}
		if strings.Contains(low, "total failed:") {
			j.Failed, _ = strconv.Atoi(strings.TrimSpace(line[strings.LastIndex(line, ":")+1:]))
		}
		if strings.Contains(low, "banned ip list:") {
			ips := strings.TrimSpace(line[strings.Index(strings.ToLower(line), "banned ip list:")+len("banned ip list:"):])
			for _, ip := range strings.Fields(ips) {
				j.Banned = append(j.Banned, ip)
			}
		}
	}
	return j
}

func unban(jail, ip string) error {
	if jail == "" || net.ParseIP(ip) == nil {
		return fmt.Errorf("jail and IP required")
	}
	if out, err := exec.Command("fail2ban-client", "set", jail, "unbanip", ip).CombinedOutput(); err != nil {
		return fmt.Errorf("unban: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func threats() *rpc.ThreatStatus {
	st := &rpc.ThreatStatus{}
	counts := map[string]int{}
	for _, logf := range []string{"/var/log/auth.log", "/var/log/secure"} {
		f, err := os.Open(logf)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		var lines []string
		for sc.Scan() {
			lines = append(lines, sc.Text())
			if len(lines) > 4000 {
				lines = lines[1:]
			}
		}
		_ = f.Close()
		for _, line := range lines {
			if !strings.Contains(strings.ToLower(line), "failed password") && !strings.Contains(strings.ToLower(line), "invalid user") {
				continue
			}
			ip := lastIP(line)
			if ip == "" {
				continue
			}
			counts[ip]++
			if len(st.FailedLogins) < 40 {
				st.FailedLogins = append(st.FailedLogins, rpc.ThreatEvent{IP: ip, Detail: truncate(line, 160), Source: filepath.Base(logf)})
			}
		}
	}
	for ip, n := range counts {
		st.TopSources = append(st.TopSources, rpc.ThreatEvent{IP: ip, Count: n, Source: "auth"})
	}
	fb := fail2banStatus()
	if fb != nil {
		for _, j := range fb.Jails {
			for _, ip := range j.Banned {
				st.Banned = append(st.Banned, rpc.ThreatEvent{IP: ip, Source: j.Name, Detail: "fail2ban banned"})
			}
		}
	}
	return st
}

func lastIP(line string) string {
	fields := strings.Fields(line)
	for i := len(fields) - 1; i >= 0; i-- {
		cand := strings.Trim(fields[i], "[]")
		if net.ParseIP(cand) != nil {
			return cand
		}
	}
	return ""
}

func ffmpegStatus() *rpc.FFmpegStatus {
	st := &rpc.FFmpegStatus{}
	b, err := exec.Command("ffmpeg", "-version").CombinedOutput()
	if err != nil {
		return st
	}
	st.Installed = true
	first := strings.SplitN(string(b), "\n", 2)[0]
	st.Version = strings.TrimSpace(first)
	cb, _ := exec.Command("ffmpeg", "-hide_banner", "-encoders").CombinedOutput()
	for _, line := range strings.Split(string(cb), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && (fields[0] == "V" || fields[0] == "A" || strings.HasPrefix(fields[0], "V") || strings.HasPrefix(fields[0], "A")) {
			if len(st.Codecs) < 40 {
				st.Codecs = append(st.Codecs, fields[1])
			}
		}
	}
	return st
}

func ffmpegConvert(src, dest, extra string) (string, error) {
	if src == "" || dest == "" {
		return "", fmt.Errorf("source and destination required")
	}
	if strings.Contains(src, "..") || strings.Contains(dest, "..") {
		return "", fmt.Errorf("invalid path")
	}
	args := []string{"-y", "-hide_banner", "-i", src}
	if extra != "" {
		args = append(args, strings.Fields(extra)...)
	}
	args = append(args, dest)
	cmd := exec.Command("ffmpeg", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("ffmpeg: %s", tail(string(out), 400))
	}
	return tail(string(out), 800), nil
}

func memcachedStatus() *rpc.MemcachedStatus {
	st := &rpc.MemcachedStatus{Listen: "127.0.0.1", Port: 11211, MemoryMB: 64}
	if _, err := exec.LookPath("memcached"); err != nil {
		return st
	}
	st.Installed = true
	st.Active = exec.Command("systemctl", "is-active", "--quiet", "memcached").Run() == nil
	b, _ := os.ReadFile("/etc/memcached.conf")
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || strings.HasPrefix(f[0], "#") {
			continue
		}
		switch f[0] {
		case "-m":
			st.MemoryMB, _ = strconv.Atoi(f[1])
		case "-l":
			st.Listen = f[1]
		case "-p":
			st.Port, _ = strconv.Atoi(f[1])
		}
	}
	c, err := net.DialTimeout("tcp", net.JoinHostPort(st.Listen, strconv.Itoa(st.Port)), 800*time.Millisecond)
	if err == nil {
		defer c.Close()
		_, _ = c.Write([]byte("stats\n"))
		_ = c.SetReadDeadline(time.Now().Add(time.Second))
		r := bufio.NewReader(c)
		st.Stats = map[string]string{}
		for {
			line, err := r.ReadString('\n')
			if err != nil || strings.HasPrefix(line, "END") {
				break
			}
			f := strings.Fields(line)
			if len(f) >= 3 && f[0] == "STAT" {
				st.Stats[f[1]] = f[2]
			}
		}
	}
	return st
}

func setMemcached(mem int, listen string, port int) error {
	if mem <= 0 {
		mem = 64
	}
	if listen == "" {
		listen = "127.0.0.1"
	}
	if port <= 0 {
		port = 11211
	}
	body := fmt.Sprintf("-d\nlogfile /var/log/memcached.log\n-m %d\n-p %d\n-u memcache\n-l %s\n", mem, port, listen)
	if err := os.WriteFile("/etc/memcached.conf", []byte(body), 0644); err != nil {
		return err
	}
	if out, err := exec.Command("systemctl", "restart", "memcached").CombinedOutput(); err != nil {
		return fmt.Errorf("memcached: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func quotaReady() bool {
	return exec.Command("quotaon", "-p", quotaFS()).Run() == nil || fileContains("/proc/mounts", "usrquota")
}

func quotaFS() string {
	b, _ := os.ReadFile("/proc/mounts")
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[1] == "/" {
			return "/"
		}
	}
	return "/"
}

func ensureQuota() error {
	if _, err := exec.LookPath("setquota"); err != nil {
		if err := software.AptInstall("quota"); err != nil {
			return fmt.Errorf("install quota: %w", err)
		}
	}
	if quotaReady() {
		return nil
	}
	_ = exec.Command("mount", "-o", "remount,usrquota,grpquota", "/").Run()
	_ = exec.Command("quotacheck", "-cum", "/").Run()
	if err := exec.Command("quotaon", "/").Run(); err != nil {
		return fmt.Errorf("cannot enable filesystem quotas on this mount (overlay/Docker often has no usrquota)")
	}
	return nil
}

func fileContains(path, needle string) bool {
	b, err := os.ReadFile(path)
	return err == nil && strings.Contains(string(b), needle)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

//go:build linux

package monitoring

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

type Collector struct {
	mu       sync.Mutex
	cpu      cpuSnap
	net      map[string]netSnap
	procs    map[int]procSnap
	users    map[string]string
	sampled  bool
	diskMu   sync.Mutex
	diskAt   time.Time
	diskBy   map[string]uint64
}

type cpuSnap struct {
	idle  uint64
	total uint64
	at    time.Time
}

type netSnap struct {
	rx uint64
	tx uint64
	at time.Time
}

type procSnap struct {
	ticks uint64
	at    time.Time
}

type procRow struct {
	pid    int
	user   string
	name   string
	cpu    float64
	memory uint64
}

func (c *Collector) Stats() *rpc.SystemStats {
	now := time.Now()
	st := &rpc.SystemStats{
		Hostname: hostname(),
		OS:       prettyOS(),
		Kernel:   readTrim("/proc/sys/kernel/osrelease"),
		Arch:     runtime.GOARCH,
		UptimeSec: uptimeSec(),
		Time:     now.UTC().Format(time.RFC3339),
		Disks:    []rpc.SystemDisk{},
		Network:  []rpc.SystemNet{},
		Top:      []rpc.SystemProc{},
		Services: []rpc.SystemService{},
		Users:    []rpc.UserUsage{},
		Listen:   []rpc.SystemListen{},
	}
	st.CPU = readCPU()
	st.Memory, st.Swap = readMemory()
	st.Load = readLoad()
	st.Disks = readDisks()

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.net == nil {
		c.net = map[string]netSnap{}
	}
	if c.procs == nil {
		c.procs = map[int]procSnap{}
	}
	if c.users == nil {
		c.users = map[string]string{}
	}

	if !c.sampled {
		c.cpu = readCPUSnap()
		c.net = readNetSnaps()
		time.Sleep(150 * time.Millisecond)
		c.sampled = true
	}

	cpuNow := readCPUSnap()
	if d := cpuNow.total - c.cpu.total; d > 0 {
		idle := cpuNow.idle - c.cpu.idle
		pct := (1 - float64(idle)/float64(d)) * 100
		if pct < 0 {
			pct = 0
		}
		if pct > 100 {
			pct = 100
		}
		st.CPU.Percent = round1(pct)
	}
	c.cpu = cpuNow

	st.Network = c.readNetwork(now)
	st.Processes, st.Top, st.Users = c.readProcesses(st.Memory.Total, now)
	st.Services = readServices()
	st.Listen = c.readListen()
	return st
}

func hostname() string {
	h, _ := os.Hostname()
	return h
}

func prettyOS() string {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return "Linux"
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"`)
		}
	}
	return "Linux"
}

func readTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func uptimeSec() int64 {
	fields := strings.Fields(readTrim("/proc/uptime"))
	if len(fields) == 0 {
		return 0
	}
	sec, _ := strconv.ParseFloat(fields[0], 64)
	return int64(sec)
}

func readCPU() rpc.SystemCPU {
	out := rpc.SystemCPU{}
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		out.Cores = runtime.NumCPU()
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "processor") {
			out.Cores++
		}
		if out.Model == "" && strings.HasPrefix(line, "model name") {
			if _, rest, ok := strings.Cut(line, ":"); ok {
				out.Model = strings.TrimSpace(rest)
			}
		}
	}
	if out.Cores == 0 {
		out.Cores = runtime.NumCPU()
	}
	return out
}

func readCPUSnap() cpuSnap {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return cpuSnap{at: time.Now()}
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return cpuSnap{at: time.Now()}
	}
	fields := strings.Fields(sc.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return cpuSnap{at: time.Now()}
	}
	var nums []uint64
	var total uint64
	for _, f := range fields[1:] {
		n, _ := strconv.ParseUint(f, 10, 64)
		nums = append(nums, n)
		total += n
	}
	idle := nums[3]
	if len(nums) > 4 {
		idle += nums[4] // iowait
	}
	return cpuSnap{idle: idle, total: total, at: time.Now()}
}

func readMemory() (rpc.SystemMemory, rpc.SystemMemory) {
	vals := map[string]uint64{}
	f, err := os.Open("/proc/meminfo")
	if err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) < 2 {
				continue
			}
			n, _ := strconv.ParseUint(fields[1], 10, 64)
			vals[strings.TrimSuffix(fields[0], ":")] = n * 1024
		}
		f.Close()
	}
	mem := rpc.SystemMemory{
		Total:     vals["MemTotal"],
		Free:      vals["MemFree"],
		Available: vals["MemAvailable"],
	}
	if mem.Available > 0 && mem.Available <= mem.Total {
		mem.Used = mem.Total - mem.Available
	} else {
		mem.Used = mem.Total - mem.Free - vals["Buffers"] - vals["Cached"]
	}
	if mem.Used > mem.Total {
		mem.Used = mem.Total
	}
	mem.Percent = pct(mem.Used, mem.Total)

	swap := rpc.SystemMemory{
		Total: vals["SwapTotal"],
		Free:  vals["SwapFree"],
	}
	if swap.Total > swap.Free {
		swap.Used = swap.Total - swap.Free
	}
	swap.Percent = pct(swap.Used, swap.Total)
	return mem, swap
}

func readLoad() rpc.SystemLoad {
	fields := strings.Fields(readTrim("/proc/loadavg"))
	out := rpc.SystemLoad{}
	if len(fields) >= 3 {
		out.One, _ = strconv.ParseFloat(fields[0], 64)
		out.Five, _ = strconv.ParseFloat(fields[1], 64)
		out.Fifteen, _ = strconv.ParseFloat(fields[2], 64)
	}
	return out
}

func readDisks() []rpc.SystemDisk {
	skipFS := map[string]bool{
		"tmpfs": true, "devtmpfs": true, "proc": true, "sysfs": true, "cgroup": true, "cgroup2": true,
		"devpts": true, "securityfs": true, "pstore": true, "bpf": true, "tracefs": true, "debugfs": true,
		"mqueue": true, "hugetlbfs": true, "fusectl": true, "configfs": true, "ramfs": true, "nsfs": true,
		"autofs": true, "binfmt_misc": true, "rpc_pipefs": true,
	}
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return nil
	}
	defer f.Close()
	seen := map[string]bool{}
	var out []rpc.SystemDisk
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 {
			continue
		}
		device, mount, fs := fields[0], fields[1], fields[2]
		if skipFS[fs] {
			continue
		}
		if fs == "overlay" && mount != "/" {
			continue
		}
		if strings.HasPrefix(mount, "/snap") || strings.HasPrefix(mount, "/run") || strings.HasPrefix(mount, "/sys") || strings.HasPrefix(mount, "/proc") || strings.HasPrefix(mount, "/dev") || strings.HasPrefix(mount, "/etc/") {
			continue
		}
		info, err := os.Stat(mount)
		if err != nil || !info.IsDir() {
			continue
		}
		if seen[mount] {
			continue
		}
		var s syscall.Statfs_t
		if err := syscall.Statfs(mount, &s); err != nil || s.Blocks == 0 {
			continue
		}
		seen[mount] = true
		total := s.Blocks * uint64(s.Bsize)
		free := s.Bavail * uint64(s.Bsize)
		used := total - s.Bfree*uint64(s.Bsize)
		out = append(out, rpc.SystemDisk{
			Mount:   mount,
			Device:  device,
			FS:      fs,
			Total:   total,
			Used:    used,
			Free:    free,
			Percent: pct(used, total),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mount < out[j].Mount })
	return out
}

func readNetSnaps() map[string]netSnap {
	out := map[string]netSnap{}
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return out
	}
	defer f.Close()
	now := time.Now()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if skipIface(name) {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) < 10 {
			continue
		}
		rx, _ := strconv.ParseUint(fields[0], 10, 64)
		tx, _ := strconv.ParseUint(fields[8], 10, 64)
		out[name] = netSnap{rx: rx, tx: tx, at: now}
	}
	return out
}

func skipIface(name string) bool {
	if name == "lo" || strings.HasPrefix(name, "veth") || strings.HasPrefix(name, "br-") {
		return true
	}
	for _, p := range []string{"gre", "gretap", "erspan", "sit", "tun", "tap", "dummy", "ip6tnl", "ip6gre", "ip6_vti", "ip_vti", "ip6_vti"} {
		if name == p || strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

func (c *Collector) readNetwork(now time.Time) []rpc.SystemNet {
	cur := readNetSnaps()
	var out []rpc.SystemNet
	for name, snap := range cur {
		row := rpc.SystemNet{Name: name, RxBytes: snap.rx, TxBytes: snap.tx}
		if prev, ok := c.net[name]; ok && now.After(prev.at) {
			sec := now.Sub(prev.at).Seconds()
			if sec > 0.05 {
				if snap.rx >= prev.rx {
					row.RxRate = uint64(float64(snap.rx-prev.rx) / sec)
				}
				if snap.tx >= prev.tx {
					row.TxRate = uint64(float64(snap.tx-prev.tx) / sec)
				}
			}
		}
		out = append(out, row)
	}
	c.net = cur
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (c *Collector) readProcesses(memTotal uint64, now time.Time) (int, []rpc.SystemProc, []rpc.UserUsage) {
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return 0, nil, nil
	}
	next := map[int]procSnap{}
	var rows []procRow
	count := 0
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		count++
		ticks, rss, uid, name := readProc(pid)
		if name == "" && rss == 0 {
			continue
		}
		cpu := 0.0
		if prev, ok := c.procs[pid]; ok && ticks >= prev.ticks {
			sec := now.Sub(prev.at).Seconds()
			if sec > 0.05 {
				// ticks are 1/100s (USER_HZ)
				cpu = (float64(ticks-prev.ticks) / 100.0) / sec * 100
				if cpu < 0 {
					cpu = 0
				}
			}
		}
		next[pid] = procSnap{ticks: ticks, at: now}
		rows = append(rows, procRow{pid: pid, user: c.uidName(uid), name: name, cpu: cpu, memory: rss})
	}
	c.procs = next
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].cpu == rows[j].cpu {
			return rows[i].memory > rows[j].memory
		}
		return rows[i].cpu > rows[j].cpu
	})
	if len(rows) > 12 {
		rows = rows[:12]
	}
	out := make([]rpc.SystemProc, 0, len(rows))
	for _, r := range rows {
		out = append(out, rpc.SystemProc{
			PID:    r.pid,
			User:   r.user,
			Name:   r.name,
			CPU:    round1(r.cpu),
			Memory: r.memory,
			MemPct: pct(r.memory, memTotal),
		})
	}
	return count, out, c.userUsage(rows, memTotal)
}

func readProc(pid int) (ticks uint64, rss uint64, uid, name string) {
	base := filepath.Join("/proc", strconv.Itoa(pid))
	statb, err := os.ReadFile(filepath.Join(base, "stat"))
	if err != nil {
		return
	}
	stat := string(statb)
	l := strings.IndexByte(stat, '(')
	r := strings.LastIndexByte(stat, ')')
	if l >= 0 && r > l {
		name = stat[l+1 : r]
		rest := strings.Fields(stat[r+1:])
		if len(rest) > 13 {
			ut, _ := strconv.ParseUint(rest[11], 10, 64)
			st, _ := strconv.ParseUint(rest[12], 10, 64)
			ticks = ut + st
		}
	}
	status, err := os.Open(filepath.Join(base, "status"))
	if err != nil {
		return
	}
	defer status.Close()
	sc := bufio.NewScanner(status)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "Uid:") {
			fields := strings.Fields(line)
			if len(fields) > 1 {
				uid = fields[1]
			}
		}
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) > 1 {
				n, _ := strconv.ParseUint(fields[1], 10, 64)
				rss = n * 1024
			}
		}
	}
	if cmd, err := os.ReadFile(filepath.Join(base, "comm")); err == nil {
		if n := strings.TrimSpace(string(cmd)); n != "" {
			name = n
		}
	}
	return
}

func (c *Collector) uidName(uid string) string {
	if uid == "" {
		return ""
	}
	if n, ok := c.users[uid]; ok {
		return n
	}
	u, err := user.LookupId(uid)
	name := uid
	if err == nil {
		name = u.Username
	}
	c.users[uid] = name
	return name
}

type sockOwner struct {
	pid  int
	uid  string
	name string
}

func (c *Collector) readListen() []rpc.SystemListen {
	owners := socketOwners()
	var out []rpc.SystemListen
	out = append(out, parseProcNet("/proc/net/tcp", "tcp", false, 0x0A, owners, c)...)
	out = append(out, parseProcNet("/proc/net/tcp6", "tcp", true, 0x0A, owners, c)...)
	out = append(out, parseProcNet("/proc/net/udp", "udp", false, -1, owners, c)...)
	out = append(out, parseProcNet("/proc/net/udp6", "udp", true, -1, owners, c)...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Port == out[j].Port {
			if out[i].Proto == out[j].Proto {
				if out[i].Address == out[j].Address {
					return out[i].PID < out[j].PID
				}
				return out[i].Address < out[j].Address
			}
			return out[i].Proto < out[j].Proto
		}
		return out[i].Port < out[j].Port
	})
	return out
}

func parseProcNet(path, proto string, v6 bool, listenState int64, owners map[uint64]sockOwner, c *Collector) []rpc.SystemListen {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return nil
	}
	var out []rpc.SystemListen
	seen := map[string]bool{}
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 10 {
			continue
		}
		if listenState >= 0 {
			st, err := strconv.ParseInt(fields[3], 16, 64)
			if err != nil || st != listenState {
				continue
			}
		} else if !strings.HasSuffix(strings.ToUpper(fields[2]), ":0000") {
			continue
		}
		ip, port, ok := parseHexAddr(fields[1], v6)
		if !ok || port == 0 {
			continue
		}
		inode, _ := strconv.ParseUint(fields[9], 10, 64)
		addr := joinAddr(ip, port)
		key := proto + "|" + addr + "|" + strconv.FormatUint(inode, 10)
		if seen[key] {
			continue
		}
		seen[key] = true
		row := rpc.SystemListen{Proto: proto, Address: addr, Port: port}
		if own, ok := owners[inode]; ok {
			row.PID = own.pid
			row.Name = own.name
			row.User = c.uidName(own.uid)
		}
		out = append(out, row)
	}
	return out
}

func parseHexAddr(s string, v6 bool) (string, int, bool) {
	host, portHex, ok := strings.Cut(s, ":")
	if !ok {
		return "", 0, false
	}
	p, err := strconv.ParseUint(portHex, 16, 16)
	if err != nil {
		return "", 0, false
	}
	return parseHexIP(host, v6), int(p), true
}

func parseHexIP(hex string, v6 bool) string {
	if !v6 {
		n, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return hex
		}
		return net.IPv4(byte(n), byte(n>>8), byte(n>>16), byte(n>>24)).String()
	}
	if len(hex) != 32 {
		return hex
	}
	b := make([]byte, 16)
	for i := 0; i < 4; i++ {
		n, err := strconv.ParseUint(hex[i*8:(i+1)*8], 16, 32)
		if err != nil {
			return hex
		}
		b[i*4] = byte(n)
		b[i*4+1] = byte(n >> 8)
		b[i*4+2] = byte(n >> 16)
		b[i*4+3] = byte(n >> 24)
	}
	return net.IP(b).String()
}

func joinAddr(ip string, port int) string {
	if strings.Contains(ip, ":") {
		return fmt.Sprintf("[%s]:%d", ip, port)
	}
	return fmt.Sprintf("%s:%d", ip, port)
}

func socketOwners() map[uint64]sockOwner {
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	out := map[uint64]sockOwner{}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		base := filepath.Join("/proc", e.Name())
		fds, err := os.ReadDir(filepath.Join(base, "fd"))
		if err != nil {
			continue
		}
		_, _, uid, name := readProc(pid)
		if name == "" {
			if b, err := os.ReadFile(filepath.Join(base, "comm")); err == nil {
				name = strings.TrimSpace(string(b))
			}
		}
		for _, fd := range fds {
			target, err := os.Readlink(filepath.Join(base, "fd", fd.Name()))
			if err != nil || !strings.HasPrefix(target, "socket:[") {
				continue
			}
			n := strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")
			inode, err := strconv.ParseUint(n, 10, 64)
			if err != nil {
				continue
			}
			if _, ok := out[inode]; !ok {
				out[inode] = sockOwner{pid: pid, uid: uid, name: name}
			}
		}
	}
	return out
}

func phpProc(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return n == "php" ||
		strings.HasPrefix(n, "php-") ||
		strings.HasPrefix(n, "php:") ||
		strings.HasPrefix(n, "php8") ||
		strings.HasPrefix(n, "php7") ||
		strings.Contains(n, "php-fpm")
}

func (c *Collector) userUsage(rows []procRow, memTotal uint64) []rpc.UserUsage {
	disk := c.homeDisks()
	type agg struct {
		procs int
		php   int
		mem   uint64
		cpu   float64
	}
	by := map[string]*agg{}
	ensure := func(name string) *agg {
		if a, ok := by[name]; ok {
			return a
		}
		a := &agg{}
		by[name] = a
		return a
	}
	for name := range disk {
		ensure(name)
	}
	ents, _ := os.ReadDir("/home")
	for _, e := range ents {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && e.Name() != "lost+found" {
			ensure(e.Name())
		}
	}
	for _, r := range rows {
		if r.user == "" {
			continue
		}
		a, ok := by[r.user]
		if !ok {
			continue
		}
		a.procs++
		a.mem += r.memory
		a.cpu += r.cpu
		if phpProc(r.name) {
			a.php++
		}
	}
	out := make([]rpc.UserUsage, 0, len(by))
	for name, a := range by {
		out = append(out, rpc.UserUsage{
			User:         name,
			Processes:    a.procs,
			PHPProcesses: a.php,
			Memory:       a.mem,
			MemPct:       pct(a.mem, memTotal),
			CPU:          round1(a.cpu),
			DiskUsed:     disk[name],
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Memory == out[j].Memory {
			if out[i].PHPProcesses == out[j].PHPProcesses {
				return out[i].User < out[j].User
			}
			return out[i].PHPProcesses > out[j].PHPProcesses
		}
		return out[i].Memory > out[j].Memory
	})
	return out
}

func (c *Collector) homeDisks() map[string]uint64 {
	c.diskMu.Lock()
	defer c.diskMu.Unlock()
	if c.diskBy != nil && time.Since(c.diskAt) < 30*time.Second {
		return c.diskBy
	}
	out := map[string]uint64{}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "du", "-b", "--max-depth=1", "/home")
	b, err := cmd.Output()
	if err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			n, _ := strconv.ParseUint(fields[0], 10, 64)
			p := filepath.Clean(fields[1])
			if p == "/home" {
				continue
			}
			out[filepath.Base(p)] = n
		}
	}
	c.diskBy = out
	c.diskAt = time.Now()
	return out
}

var watchServices = []struct{ unit, title string }{
	{"nginx", "Nginx"},
	{"apache2", "Apache"},
	{"mysql", "MySQL"},
	{"mariadb", "MariaDB"},
	{"redis-server", "Redis"},
	{"ssh", "OpenSSH"},
	{"vsftpd", "FTP"},
	{"clamav-daemon", "ClamAV"},
	{"php8.1-fpm", "PHP 8.1 FPM"},
	{"php8.2-fpm", "PHP 8.2 FPM"},
	{"php8.3-fpm", "PHP 8.3 FPM"},
	{"php8.4-fpm", "PHP 8.4 FPM"},
	{"siroc-panel", "Siroc Panel"},
	{"siroc-agent", "Siroc Agent"},
}

func readServices() []rpc.SystemService {
	args := []string{"show", "--property=Id,LoadState,ActiveState", "--no-page"}
	for _, s := range watchServices {
		args = append(args, s.unit+".service")
	}
	out, err := exec.Command("systemctl", args...).Output()
	if err != nil {
		return nil
	}
	blocks := strings.Split(strings.TrimSpace(string(out)), "\n\n")
	info := map[string]struct{ load, active string }{}
	for _, b := range blocks {
		id, load, active := "", "", ""
		for _, line := range strings.Split(b, "\n") {
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			switch k {
			case "Id":
				id = strings.TrimSuffix(v, ".service")
			case "LoadState":
				load = v
			case "ActiveState":
				active = v
			}
		}
		if id != "" {
			info[id] = struct{ load, active string }{load, active}
		}
	}
	var list []rpc.SystemService
	for _, s := range watchServices {
		st, ok := info[s.unit]
		if !ok || st.load == "not-found" || st.load == "masked" {
			continue
		}
		list = append(list, rpc.SystemService{Name: s.unit, Title: s.title, Active: st.active == "active"})
	}
	return list
}

func pct(used, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return round1(float64(used) / float64(total) * 100)
}

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}

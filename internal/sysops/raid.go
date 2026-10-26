package sysops

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
)

var raidDevRe = regexp.MustCompile(`^/dev/(sd[a-z]+[0-9]*|hd[a-z]+[0-9]*|vd[a-z]+[0-9]*|xvd[a-z]+[0-9]*|nvme[0-9]+n[0-9]+(p[0-9]+)?)$`)
var raidNameRe = regexp.MustCompile(`^md[0-9]{1,3}$`)

func RAIDMinDisks(level string) int {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "0", "raid0", "1", "raid1":
		return 2
	case "5", "raid5":
		return 3
	case "10", "raid10":
		return 4
	default:
		return 0
	}
}

func NormalizeRAIDLevel(level string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "0", "raid0":
		return "0", nil
	case "1", "raid1":
		return "1", nil
	case "5", "raid5":
		return "5", nil
	case "10", "raid10":
		return "10", nil
	default:
		return "", fmt.Errorf("RAID level must be 0, 1, 5, or 10")
	}
}

func ValidRAIDDevice(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" || strings.Contains(path, "..") || strings.ContainsAny(path, " \t\r\n") {
		return false
	}
	return raidDevRe.MatchString(path)
}

func ValidRAIDName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return true
	}
	return raidNameRe.MatchString(name)
}

type lsblkTree struct {
	Blockdevices []lsblkDev `json:"blockdevices"`
}

type lsblkDev struct {
	Name        string      `json:"name"`
	Path        string      `json:"path"`
	Type        string      `json:"type"`
	Size        json.Number `json:"size"`
	Fstype      *string     `json:"fstype"`
	Mountpoint  *string     `json:"mountpoint"`
	Mountpoints []any       `json:"mountpoints"`
	Model       string      `json:"model"`
	Tran        string      `json:"tran"`
	Children    []lsblkDev  `json:"children"`
}

func (d lsblkDev) devPath() string {
	p := strings.TrimSpace(d.Path)
	if p != "" {
		return p
	}
	n := strings.TrimSpace(d.Name)
	if n == "" {
		return ""
	}
	if strings.HasPrefix(n, "/dev/") {
		return n
	}
	return "/dev/" + n
}

func (d lsblkDev) sizeBytes() int64 {
	n, _ := d.Size.Int64()
	return n
}

func (d lsblkDev) mounts() []string {
	var out []string
	if d.Mountpoint != nil {
		if m := strings.TrimSpace(*d.Mountpoint); m != "" && m != "null" {
			out = append(out, m)
		}
	}
	for _, raw := range d.Mountpoints {
		s, ok := raw.(string)
		if !ok {
			continue
		}
		s = strings.TrimSpace(s)
		if s == "" || s == "null" {
			continue
		}
		out = append(out, s)
	}
	return out
}

func (d lsblkDev) isRAID() bool {
	t := strings.ToLower(d.Type)
	return strings.HasPrefix(t, "raid") || t == "md"
}

func (d lsblkDev) member() bool {
	return strings.EqualFold(d.fsType(), "linux_raid_member") || d.isRAID()
}

func (d lsblkDev) fsType() string {
	if d.Fstype == nil {
		return ""
	}
	return strings.TrimSpace(*d.Fstype)
}

func (d lsblkDev) busy() bool {
	if len(d.mounts()) > 0 || d.member() {
		return true
	}
	if fs := d.fsType(); fs != "" && !strings.EqualFold(fs, "linux_raid_member") {
		return true
	}
	for _, c := range d.Children {
		if c.busy() {
			return true
		}
	}
	return false
}

func (d lsblkDev) system() bool {
	for _, m := range d.mounts() {
		switch m {
		case "/", "/boot", "/boot/efi", "/usr", "/var", "/home":
			return true
		}
	}
	for _, c := range d.Children {
		if c.system() {
			return true
		}
	}
	return false
}

func ClassifyBlockDevices(raw []byte, mdstat string) *rpc.RAIDStatus {
	st := &rpc.RAIDStatus{Arrays: []rpc.RAIDArray{}, Disks: []rpc.RAIDDisk{}}
	var tree lsblkTree
	if err := json.Unmarshal(raw, &tree); err != nil {
		st.Message = "could not read disks"
		return st
	}
	arrays := map[string]*rpc.RAIDArray{}
	var walk func(d lsblkDev, system bool)
	walk = func(d lsblkDev, system bool) {
		system = system || d.system()
		path := d.devPath()
		if d.isRAID() && ValidRAIDName(strings.TrimPrefix(path, "/dev/")) {
			row := arrays[path]
			if row == nil {
				row = &rpc.RAIDArray{Device: path, Level: d.Type, Size: d.sizeBytes()}
				arrays[path] = row
			}
			if row.Size == 0 {
				row.Size = d.sizeBytes()
			}
		}
		if !system && !d.busy() && (d.Type == "disk" || d.Type == "part") && ValidRAIDDevice(path) {
			if d.Type == "disk" && len(d.Children) > 0 {
				// Offer partitions instead of a disk that already has a table.
			} else {
				st.Disks = append(st.Disks, rpc.RAIDDisk{
					Path:      path,
					Name:      d.Name,
					Type:      d.Type,
					Size:      d.sizeBytes(),
					Model:     strings.TrimSpace(d.Model),
					Transport: strings.TrimSpace(d.Tran),
				})
			}
		}
		for _, c := range d.Children {
			walk(c, system)
		}
	}
	for _, d := range tree.Blockdevices {
		walk(d, false)
	}
	for name, info := range ParseMDStat(mdstat) {
		dev := "/dev/" + name
		row := arrays[dev]
		if row == nil {
			row = &rpc.RAIDArray{Device: dev}
			arrays[dev] = row
		}
		row.State = info.state
		if info.level != "" {
			row.Level = info.level
		}
		row.Devices = info.members
	}
	for _, row := range arrays {
		st.Arrays = append(st.Arrays, *row)
	}
	return st
}

func ParseMDStat(text string) map[string]mdLine {
	out := map[string]mdLine{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "md") {
			continue
		}
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if !raidNameRe.MatchString(name) {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) < 2 {
			continue
		}
		info := mdLine{state: fields[0], level: fields[1]}
		for _, f := range fields[2:] {
			base := f
			if i := strings.IndexByte(base, '['); i >= 0 {
				base = base[:i]
			}
			base = strings.TrimSuffix(base, "(S)")
			base = strings.TrimSuffix(base, "(F)")
			if base == "" {
				continue
			}
			path := base
			if !strings.HasPrefix(path, "/dev/") {
				path = "/dev/" + path
			}
			info.members = append(info.members, path)
		}
		out[name] = info
	}
	return out
}

type mdLine struct {
	state   string
	level   string
	members []string
}

func PickRAIDName(used map[string]bool, want string) (string, error) {
	want = strings.TrimSpace(want)
	if want != "" {
		if !ValidRAIDName(want) {
			return "", fmt.Errorf("array name must look like md0")
		}
		if used[want] {
			return "", fmt.Errorf("%s already exists", want)
		}
		return want, nil
	}
	for i := 0; i < 32; i++ {
		name := "md" + strconv.Itoa(i)
		if !used[name] {
			return name, nil
		}
	}
	return "", fmt.Errorf("no free md device")
}

func CheckRAIDCreate(level string, devices []string, unused []rpc.RAIDDisk, arrays []rpc.RAIDArray) (string, string, []string, error) {
	lv, err := NormalizeRAIDLevel(level)
	if err != nil {
		return "", "", nil, err
	}
	need := RAIDMinDisks(lv)
	if len(devices) < need {
		return "", "", nil, fmt.Errorf("RAID %s needs at least %d disks", lv, need)
	}
	if lv == "10" && len(devices)%2 != 0 {
		return "", "", nil, fmt.Errorf("RAID 10 needs an even number of disks")
	}
	if len(devices) > 16 {
		return "", "", nil, fmt.Errorf("select at most 16 disks")
	}
	allow := map[string]struct{}{}
	for _, d := range unused {
		allow[d.Path] = struct{}{}
	}
	seen := map[string]struct{}{}
	var clean []string
	for _, d := range devices {
		d = strings.TrimSpace(d)
		if !ValidRAIDDevice(d) {
			return "", "", nil, fmt.Errorf("invalid disk %s", d)
		}
		if _, ok := seen[d]; ok {
			return "", "", nil, fmt.Errorf("disk listed twice")
		}
		if _, ok := allow[d]; !ok {
			return "", "", nil, fmt.Errorf("%s is in use or is the system disk", d)
		}
		seen[d] = struct{}{}
		clean = append(clean, d)
	}
	used := map[string]bool{}
	for _, a := range arrays {
		used[strings.TrimPrefix(a.Device, "/dev/")] = true
	}
	name, err := PickRAIDName(used, "")
	if err != nil {
		return "", "", nil, err
	}
	return lv, name, clean, nil
}

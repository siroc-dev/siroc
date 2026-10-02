package monitoring

import (
	"strings"
	"time"
)

type diskSnap struct {
	name   string
	reads  uint64
	rsect  uint64
	writes uint64
	wsect  uint64
	ioms   uint64
	at     time.Time
}

func parseDiskstats(raw string) []diskSnap {
	var out []diskSnap
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 14 {
			continue
		}
		name := fields[2]
		if !isWholeDisk(name) {
			continue
		}
		out = append(out, diskSnap{
			name:   name,
			reads:  parseUint(fields[3]),
			rsect:  parseUint(fields[5]),
			writes: parseUint(fields[7]),
			wsect:  parseUint(fields[9]),
			ioms:   parseUint(fields[12]),
		})
	}
	return out
}

func isWholeDisk(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return false
	}
	for _, p := range []string{"loop", "ram", "sr", "fd", "dm-", "md", "zram", "nbd"} {
		if n == p || strings.HasPrefix(n, p) {
			return false
		}
	}
	if strings.Contains(n, "nvme") || strings.HasPrefix(n, "mmcblk") {
		return !strings.Contains(n, "p")
	}
	last := n[len(n)-1]
	return last < '0' || last > '9'
}

func parseUint(s string) uint64 {
	var n uint64
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + uint64(c-'0')
	}
	return n
}

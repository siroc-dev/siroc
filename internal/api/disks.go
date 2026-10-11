package api

import (
	"sort"

	"github.com/siroc-dev/siroc/internal/rpc"
)

// dashboardDisks keeps one gauge per filesystem. A second mount of the same
// device, such as a bind mount, stays on that disk instead of appearing twice.
func dashboardDisks(in []rpc.SystemDisk) []rpc.SystemDisk {
	best := map[string]rpc.SystemDisk{}
	for _, d := range in {
		if d.Mount == "" || d.Total == 0 {
			continue
		}
		key := d.Device
		if key == "" {
			key = d.Mount
		}
		prev, ok := best[key]
		if !ok || preferDiskMount(d.Mount, prev.Mount) {
			best[key] = d
		}
	}
	out := make([]rpc.SystemDisk, 0, len(best))
	for _, d := range best {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Mount == "/" {
			return true
		}
		if out[j].Mount == "/" {
			return false
		}
		return out[i].Mount < out[j].Mount
	})
	return out
}

func preferDiskMount(next, current string) bool {
	if next == "/" {
		return true
	}
	if current == "/" {
		return false
	}
	if len(next) != len(current) {
		return len(next) < len(current)
	}
	return next < current
}

//go:build linux

package software

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func ServiceUsage(units []string) map[string]rpc.ServiceUsage {
	clean := make([]string, 0, len(units))
	seen := map[string]struct{}{}
	for _, u := range units {
		u = strings.TrimSpace(strings.TrimSuffix(u, ".service"))
		if !ValidServiceUnit(u) {
			continue
		}
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		clean = append(clean, u)
		if len(clean) >= 64 {
			break
		}
	}
	if len(clean) == 0 {
		return map[string]rpc.ServiceUsage{}
	}
	args := []string{"show", "--property=Id,MemoryCurrent,TasksCurrent,NTasks,ControlGroup", "--no-page"}
	for _, u := range clean {
		args = append(args, u+".service")
	}
	out, err := exec.Command("systemctl", args...).Output()
	if err != nil {
		return map[string]rpc.ServiceUsage{}
	}
	text := string(out)
	got := ParseServiceUsage(text)
	groups := ParseControlGroups(text)
	for id, cg := range groups {
		row := got[id]
		row.Unit = id
		if n := cgroupProcs(cg); n > 0 {
			row.Processes = n
		}
		if row.Memory == 0 {
			row.Memory = cgroupMemory(cg)
		}
		got[id] = row
	}
	return got
}

func cgroupMemory(cg string) int64 {
	for _, p := range cgroupFiles(cg, "memory.current", "memory.usage_in_bytes") {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
		if err == nil && n > 0 {
			return n
		}
	}
	return 0
}

func cgroupProcs(cg string) int {
	for _, p := range cgroupFiles(cg, "cgroup.procs") {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		n := 0
		for _, line := range strings.Split(string(b), "\n") {
			if strings.TrimSpace(line) != "" {
				n++
			}
		}
		if n > 0 {
			return n
		}
	}
	return 0
}

func cgroupFiles(cg string, names ...string) []string {
	cg = strings.Trim(filepath.ToSlash(strings.TrimSpace(cg)), "/")
	if cg == "" || strings.Contains(cg, "..") {
		return nil
	}
	roots := []string{"/sys/fs/cgroup", "/sys/fs/cgroup/memory", "/sys/fs/cgroup/systemd"}
	var out []string
	for _, root := range roots {
		base := filepath.Join(root, filepath.FromSlash(cg))
		for _, name := range names {
			out = append(out, filepath.Join(base, name))
		}
	}
	return out
}

package software

import (
	"strconv"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func ValidServiceUnit(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 64 {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-' || r == '_' || r == '.' || r == '@' || r == ':':
		default:
			return false
		}
	}
	return true
}

func ParseServiceUsage(text string) map[string]rpc.ServiceUsage {
	out := map[string]rpc.ServiceUsage{}
	for _, block := range strings.Split(text, "\n\n") {
		id, mem, tasks := "", int64(0), 0
		for _, line := range strings.Split(block, "\n") {
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			switch k {
			case "Id":
				id = strings.TrimSuffix(strings.TrimSpace(v), ".service")
			case "MemoryCurrent":
				n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
				if err == nil && n > 0 {
					mem = n
				}
			case "TasksCurrent", "NTasks":
				n, err := strconv.Atoi(strings.TrimSpace(v))
				if err == nil && n > 0 {
					tasks = n
				}
			}
		}
		if id == "" || !ValidServiceUnit(id) {
			continue
		}
		out[id] = rpc.ServiceUsage{Unit: id, Memory: mem, Processes: tasks}
	}
	return out
}

func ParseControlGroups(text string) map[string]string {
	out := map[string]string{}
	for _, block := range strings.Split(text, "\n\n") {
		id, cg := "", ""
		for _, line := range strings.Split(block, "\n") {
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			switch k {
			case "Id":
				id = strings.TrimSuffix(strings.TrimSpace(v), ".service")
			case "ControlGroup":
				cg = strings.TrimSpace(v)
			}
		}
		if id != "" && cg != "" && cg != "/" {
			out[id] = cg
		}
	}
	return out
}

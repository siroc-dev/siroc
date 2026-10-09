package sysops

import (
	"fmt"
	"strconv"
	"strings"
)

type vlanDev struct {
	id     int
	parent string
}

func vlanIfaceName(parent string, id int) (string, error) {
	if err := validIface(parent); err != nil {
		return "", err
	}
	if id < 1 || id > 4094 {
		return "", fmt.Errorf("VLAN ID must be from 1 to 4094")
	}
	suffix := "." + strconv.Itoa(id)
	name := parent + suffix
	if len(name) <= 15 {
		return name, nil
	}
	keep := 15 - len(suffix)
	if keep < 1 {
		return "", fmt.Errorf("interface name is too long for VLAN %d", id)
	}
	return parent[:keep] + suffix, nil
}

func validIface(name string) error {
	if name == "" || name == "lo" || len(name) > 15 {
		return fmt.Errorf("invalid interface")
	}
	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_' || c == '.' || c == '-' || c == ':':
		default:
			return fmt.Errorf("invalid interface")
		}
	}
	return nil
}

func parseVLANConfig(text string) map[string]vlanDev {
	out := map[string]vlanDev{}
	for _, line := range strings.Split(text, "\n") {
		if !strings.Contains(line, "|") || strings.Contains(line, "VLAN Dev name") || strings.HasPrefix(strings.TrimSpace(line), "Name-Type:") {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 3 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		id, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		parent := strings.TrimSpace(parts[2])
		if err != nil || name == "" || parent == "" || id < 1 || id > 4094 {
			continue
		}
		out[name] = vlanDev{id: id, parent: parent}
	}
	return out
}

//go:build linux

package security

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

const (
	ufwWhiteFile = "/var/lib/siroc/ufw-whitelist"
	ufwBlackFile = "/var/lib/siroc/ufw-blacklist"
	ufwMarker    = "/var/lib/siroc/ufw-on"
)

// EnsureFirewall turns the firewall on the first time UFW is available.
// A later disable from the panel is left off.
func EnsureFirewall() error {
	if _, err := exec.LookPath("ufw"); err != nil {
		return nil
	}
	if _, err := os.Stat(ufwMarker); err == nil {
		if firewallUp() {
			return applyAddressRules()
		}
		return nil
	}
	if err := turnFirewallOn(); err != nil {
		return err
	}
	return os.WriteFile(ufwMarker, []byte("1\n"), 0644)
}

func turnFirewallOn() error {
	if _, err := exec.LookPath("ufw"); err != nil {
		return fmt.Errorf("ufw is not installed")
	}
	if out, err := exec.Command("ufw", "default", "deny", "incoming").CombinedOutput(); err != nil {
		return fmt.Errorf("ufw default deny: %s", strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("ufw", "default", "allow", "outgoing").CombinedOutput(); err != nil {
		return fmt.Errorf("ufw default allow: %s", strings.TrimSpace(string(out)))
	}
	ApplyDefaultUFW()
	if err := applyAddressRules(); err != nil {
		return err
	}
	out, err := exec.Command("ufw", "--force", "enable").CombinedOutput()
	if err != nil {
		return fmt.Errorf("ufw enable: %s", strings.TrimSpace(string(out)))
	}
	return os.WriteFile(ufwMarker, []byte("1\n"), 0644)
}

func firewallUp() bool {
	out, err := exec.Command("ufw", "status").CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), "status: active")
}

func SetFirewallAddress(list, op, address string) error {
	path, other, err := firewallListPaths(list)
	if err != nil {
		return err
	}
	one, err := NormalizeIgnore([]string{address})
	if err != nil {
		return err
	}
	if len(one) != 1 {
		return fmt.Errorf("enter an address")
	}
	addr := one[0]
	switch op {
	case "add":
		others, err := loadFirewallList(other)
		if err != nil {
			return err
		}
		for _, item := range others {
			if item == addr {
				return fmt.Errorf("%s is already on the other list", addr)
			}
		}
		cur, err := loadFirewallList(path)
		if err != nil {
			return err
		}
		cur, err = NormalizeIgnore(append(cur, addr))
		if err != nil {
			return err
		}
		if err := saveFirewallList(path, cur); err != nil {
			return err
		}
	case "del":
		cur, err := loadFirewallList(path)
		if err != nil {
			return err
		}
		next := cur[:0]
		for _, item := range cur {
			if item != addr {
				next = append(next, item)
			}
		}
		if err := saveFirewallList(path, next); err != nil {
			return err
		}
	default:
		return fmt.Errorf("op must be add or del")
	}
	return applyAddressRules()
}

func firewallLists() (white, black []string) {
	white, _ = loadFirewallList(ufwWhiteFile)
	black, _ = loadFirewallList(ufwBlackFile)
	if white == nil {
		white = []string{}
	}
	if black == nil {
		black = []string{}
	}
	return white, black
}

func firewallListPaths(list string) (string, string, error) {
	switch list {
	case "whitelist":
		return ufwWhiteFile, ufwBlackFile, nil
	case "blacklist":
		return ufwBlackFile, ufwWhiteFile, nil
	default:
		return "", "", fmt.Errorf("list must be whitelist or blacklist")
	}
}

func loadFirewallList(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	var lines []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return []string{}, nil
	}
	return NormalizeIgnore(lines)
}

func saveFirewallList(path string, addrs []string) error {
	if err := os.MkdirAll("/var/lib/siroc", 0750); err != nil {
		return err
	}
	body := ""
	if len(addrs) > 0 {
		body = strings.Join(addrs, "\n") + "\n"
	}
	return os.WriteFile(path, []byte(body), 0640)
}

func applyAddressRules() error {
	if _, err := exec.LookPath("ufw"); err != nil {
		return fmt.Errorf("ufw is not installed")
	}
	out, err := exec.Command("ufw", "status", "numbered").CombinedOutput()
	if err != nil && !strings.Contains(strings.ToLower(string(out)), "status:") {
		return fmt.Errorf("ufw status: %s", strings.TrimSpace(string(out)))
	}
	ids := FirewallCommentIDs(string(out))
	sort.Sort(sort.Reverse(sort.IntSlice(ids)))
	for _, id := range ids {
		del, err := exec.Command("ufw", "--force", "delete", strconv.Itoa(id)).CombinedOutput()
		if err != nil {
			return fmt.Errorf("ufw delete: %s", strings.TrimSpace(string(del)))
		}
	}
	_, black := firewallLists()
	white, _ := firewallLists()
	for _, addr := range black {
		if err := ufwInsert("deny", addr, "siroc-blacklist"); err != nil {
			return err
		}
	}
	for _, addr := range white {
		if err := ufwInsert("allow", addr, "siroc-whitelist"); err != nil {
			return err
		}
	}
	return nil
}

func ufwInsert(action, addr, comment string) error {
	out, err := exec.Command("ufw", "insert", "1", action, "from", addr, "comment", comment).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ufw %s %s: %s", action, addr, strings.TrimSpace(string(out)))
	}
	return nil
}

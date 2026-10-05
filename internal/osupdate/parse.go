package osupdate

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
)

var upgradableRE = regexp.MustCompile(`^(\S+)/(\S+)\s+(\S+)\s+\S+\s+\[upgradable from:\s*([^\]]+)\]`)

// ParseUpgradable reads `apt list --upgradable` output.
func ParseUpgradable(out string) []rpc.OSPackage {
	var pkgs []rpc.OSPackage
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		m := upgradableRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		pkgs = append(pkgs, rpc.OSPackage{
			Name:      m[1],
			Source:    m[2],
			Available: m[3],
			Current:   strings.TrimSpace(m[4]),
		})
	}
	if pkgs == nil {
		pkgs = []rpc.OSPackage{}
	}
	return pkgs
}

func Summarize(n int) string {
	switch n {
	case 0:
		return "No packages to upgrade."
	case 1:
		return "1 package can be upgraded."
	default:
		return fmt.Sprintf("%d packages can be upgraded.", n)
	}
}

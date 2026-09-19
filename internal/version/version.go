package version

import (
	"os"
	"strings"
)

// Set at build time: -ldflags "-X github.com/siroc-dev/siroc/internal/version.Version=0.2.0"
var (
	Version = "dev"
	Commit  = ""
)

func Current() string {
	if v := strings.TrimSpace(Version); v != "" && v != "dev" {
		return v
	}
	for _, p := range []string{
		os.Getenv("SIROC_VERSION_FILE"),
		"/opt/siroc/VERSION",
		"/var/lib/siroc/version",
	} {
		if p == "" {
			continue
		}
		if b, err := os.ReadFile(p); err == nil {
			if v := strings.TrimSpace(string(b)); v != "" {
				return v
			}
		}
	}
	if Version != "" {
		return Version
	}
	return "dev"
}

func Compare(a, b string) int {
	as := parts(a)
	bs := parts(b)
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		var x, y int
		if i < len(as) {
			x = as[i]
		}
		if i < len(bs) {
			y = bs[i]
		}
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
	}
	return 0
}

func parts(v string) []int {
	v = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(v)), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	var out []int
	for _, p := range strings.Split(v, ".") {
		n := 0
		for _, c := range p {
			if c < '0' || c > '9' {
				break
			}
			n = n*10 + int(c-'0')
		}
		out = append(out, n)
	}
	return out
}

package monitoring

import (
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
)

// SiteDiskPaths returns document roots that sit outside each account home.
// A path already inside that home is omitted, because du of the home counts it.
// A path contained by another returned path is omitted so the parent is counted once.
func SiteDiskPaths(homeRoot string, items []rpc.SiteDiskItem) map[string][]string {
	homeRoot = slashClean(homeRoot)
	if homeRoot == "" || homeRoot == "/" {
		homeRoot = "/home"
	}
	raw := map[string][]string{}
	for _, it := range items {
		user := strings.TrimSpace(it.User)
		p := slashClean(it.Path)
		if user == "" || p == "" || p == "/" || !strings.HasPrefix(p, "/") {
			continue
		}
		if strings.Contains(user, "/") || strings.Contains(user, "..") {
			continue
		}
		if withinDir(homeRoot+"/"+user, p) {
			continue
		}
		raw[user] = append(raw[user], p)
	}
	out := map[string][]string{}
	for user, paths := range raw {
		kept := dedupeContained(paths)
		if len(kept) > 0 {
			out[user] = kept
		}
	}
	return out
}

func siteDiskKey(grouped map[string][]string) string {
	users := make([]string, 0, len(grouped))
	for u := range grouped {
		users = append(users, u)
	}
	sort.Strings(users)
	var b strings.Builder
	for _, u := range users {
		for _, p := range grouped[u] {
			b.WriteString(u)
			b.WriteByte('\t')
			b.WriteString(p)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// parseDu reads GNU du -sb lines ("bytes<tab>path") into cleaned path -> bytes.
func parseDu(out string) map[string]uint64 {
	sizes := map[string]uint64{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		i := strings.IndexAny(line, " \t")
		if i <= 0 {
			continue
		}
		n, err := strconv.ParseUint(line[:i], 10, 64)
		if err != nil {
			continue
		}
		p := slashClean(line[i+1:])
		if p == "" {
			continue
		}
		sizes[p] = n
	}
	return sizes
}

func dedupeContained(paths []string) []string {
	uniq := map[string]struct{}{}
	for _, p := range paths {
		uniq[p] = struct{}{}
	}
	list := make([]string, 0, len(uniq))
	for p := range uniq {
		list = append(list, p)
	}
	sort.Strings(list)
	kept := make([]string, 0, len(list))
	for _, p := range list {
		inside := false
		for _, parent := range kept {
			if withinDir(parent, p) {
				inside = true
				break
			}
		}
		if !inside {
			kept = append(kept, p)
		}
	}
	return kept
}

func withinDir(root, child string) bool {
	root = slashClean(root)
	child = slashClean(child)
	if root == "" || root == "/" || child == "" || child == "/" {
		return false
	}
	return child == root || strings.HasPrefix(child, root+"/")
}

func slashClean(p string) string {
	p = strings.TrimSpace(p)
	p = strings.ReplaceAll(p, "\\", "/")
	if p == "" {
		return ""
	}
	abs := strings.HasPrefix(p, "/")
	p = path.Clean(p)
	if abs && !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if p == "." {
		return ""
	}
	return p
}

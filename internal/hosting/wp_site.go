package hosting

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/siroc-dev/siroc/internal/layout"
	"github.com/siroc-dev/siroc/internal/rpc"
)

var wpSiteCacheDir = filepath.Join(layout.Cache, "wp-sites")

type wpSiteCacheFile struct {
	At      time.Time      `json:"at"`
	Name    string         `json:"name"`
	URL     string         `json:"url"`
	Home    string         `json:"home"`
	Version string         `json:"version"`
	Prefix  string         `json:"prefix"`
	Plugins []rpc.WPPlugin `json:"plugins"`
	WPCmds  []rpc.WPCmd    `json:"wpCmds"`
}

func (c wpSiteCacheFile) status() *rpc.WPStatus {
	plugins := c.Plugins
	if plugins == nil {
		plugins = []rpc.WPPlugin{}
	}
	cmds := c.WPCmds
	if cmds == nil {
		cmds = []rpc.WPCmd{}
	}
	return &rpc.WPStatus{
		Name:     c.Name,
		URL:      c.URL,
		Home:     c.Home,
		Version:  c.Version,
		Prefix:   c.Prefix,
		Plugins:  plugins,
		WPCmds:   cmds,
		CachedAt: c.At.UTC().Format(time.RFC3339),
	}
}

func wpSiteCachePath(user, root string) string {
	sum := wpCacheKey(user, root)
	return filepath.Join(wpSiteCacheDir, sum+".json")
}

func readWPSiteCache(user, root string) (wpSiteCacheFile, bool) {
	b, err := os.ReadFile(wpSiteCachePath(user, root))
	if err != nil {
		return wpSiteCacheFile{}, false
	}
	var c wpSiteCacheFile
	if json.Unmarshal(b, &c) != nil || c.At.IsZero() {
		return wpSiteCacheFile{}, false
	}
	return c, true
}

func loadWPSiteCache(user, root string, now time.Time) (wpSiteCacheFile, bool) {
	c, ok := readWPSiteCache(user, root)
	if !ok || !wpUserCacheFresh(c.At, now) {
		return wpSiteCacheFile{}, false
	}
	return c, true
}

func saveWPSiteCache(user, root string, c wpSiteCacheFile) error {
	if c.Plugins == nil {
		c.Plugins = []rpc.WPPlugin{}
	}
	if c.WPCmds == nil {
		c.WPCmds = []rpc.WPCmd{}
	}
	c.At = c.At.UTC()
	if err := os.MkdirAll(wpSiteCacheDir, 0700); err != nil {
		return err
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(wpSiteCachePath(user, root), b, 0600)
}

func dropWPSiteCache(user, root string) {
	_ = os.Remove(wpSiteCachePath(user, root))
}

package siteopts

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/siroc-dev/siroc/internal/validate"
)

// Cache is nginx proxy_cache for pages this site fetches from Apache or a proxy.
// An empty path uses a private directory under /var/cache/nginx/siroc/<domain>.
// Enabled is the stored flag. A path without a flag still means on, so a client
// that drops the boolean still keeps the cache.
type Cache struct {
	On    bool   `json:"enabled,omitempty"`
	Valid string `json:"valid,omitempty"`
	Path  string `json:"path,omitempty"`
}

func (c *Cache) UnmarshalJSON(data []byte) error {
	var raw struct {
		On      *bool  `json:"on"`
		Enabled *bool  `json:"enabled"`
		Valid   string `json:"valid"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.Valid = raw.Valid
	c.Path = raw.Path
	switch {
	case raw.Enabled != nil:
		c.On = *raw.Enabled
	case raw.On != nil:
		c.On = *raw.On
	case strings.TrimSpace(raw.Path) != "":
		c.On = true
	}
	return nil
}

var cacheValids = map[string]string{
	"":    "",
	"10m": "10m",
	"30m": "30m",
	"1h":  "",
	"6h":  "6h",
	"12h": "12h",
	"1d":  "1d",
	"7d":  "7d",
}

func normalizeSiteCache(in *Cache) (*Cache, error) {
	if in == nil {
		return nil, nil
	}
	valid, ok := cacheValids[strings.TrimSpace(in.Valid)]
	if !ok {
		return nil, fmt.Errorf("cache time must be 10m, 30m, 1h, 6h, 12h, 1d, or 7d")
	}
	pth, err := normalizeCachePath(in.Path)
	if err != nil {
		return nil, err
	}
	if !in.On && pth == "" && valid == "" {
		return nil, nil
	}
	return &Cache{On: in.On, Valid: valid, Path: pth}, nil
}

// CacheDir is the directory this site is allowed to fill and empty.
func CacheDir(o Options, domain string) (string, error) {
	raw := ""
	if o.Cache != nil {
		raw = o.Cache.Path
	}
	if strings.TrimSpace(raw) == "" {
		return defaultSiteCacheDir(domain)
	}
	return CachePurgePath(raw)
}

func defaultSiteCacheDir(domain string) (string, error) {
	host, err := validate.ASCIIHost(domain)
	if err != nil {
		return "", err
	}
	return CachePurgePath("/var/cache/nginx/siroc/" + host)
}

// CachePurgePath accepts a site cache directory and refuses the shared parent
// that holds every site cache.
func CachePurgePath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == DefaultProxyCachePath {
		return "", fmt.Errorf("cache path is not a directory this site can clear")
	}
	abs, err := normalizeCachePath(raw)
	if err != nil || abs == "" {
		return "", fmt.Errorf("cache path is not a directory this site can clear")
	}
	if abs == "/var/cache/nginx" || abs == DefaultProxyCachePath {
		return "", fmt.Errorf("cache path is not a directory this site can clear")
	}
	return abs, nil
}

// SiteCache reports the directory, zone, and lifetime when this site caches.
func SiteCache(o Options, domain string) (dir, zone, valid string, err error) {
	if o.Cache == nil || !o.Cache.On {
		return "", "", "", nil
	}
	dir, err = CacheDir(o, domain)
	if err != nil {
		return "", "", "", err
	}
	valid = o.Cache.Valid
	if valid == "" {
		valid = "1h"
	}
	return dir, ProxyCacheZone(dir), valid, nil
}

// CacheDirectives are the nginx lines placed inside a proxy location.
func CacheDirectives(zone, valid string) string {
	if zone == "" {
		return ""
	}
	if valid == "" {
		valid = "1h"
	}
	return fmt.Sprintf("        proxy_cache %s;\n", zone) +
		"        proxy_cache_key $scheme$host$request_uri;\n" +
		fmt.Sprintf("        proxy_cache_valid 200 301 302 %s;\n", valid) +
		"        proxy_cache_lock on;\n" +
		"        proxy_ignore_headers Set-Cookie Cache-Control Expires X-Accel-Expires;\n" +
		"        proxy_cache_bypass $http_authorization;\n" +
		"        add_header X-Cache-Status $upstream_cache_status always;\n"
}

// InsertCache adds cache lines to a generated proxy location that does not already cache.
func InsertCache(loc, lines string) string {
	if lines == "" || strings.Contains(loc, "proxy_cache ") {
		return loc
	}
	const end = "\n    }\n"
	idx := strings.LastIndex(loc, end)
	if idx < 0 {
		return loc
	}
	return loc[:idx+1] + lines + loc[idx+1:]
}

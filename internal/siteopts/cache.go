package siteopts

import (
	"encoding/json"
	"fmt"
	"regexp"
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
	// Preset is the recipe the form last applied: public, wordpress, app, long, or custom.
	Preset string `json:"preset,omitempty"`
	// Size is the disk cap (proxy_cache_path max_size). Empty means 1g.
	Size string `json:"size,omitempty"`
	// Idle is how long an unused file stays (inactive). Empty means 1d.
	Idle string `json:"idle,omitempty"`
	// Stale serves the saved page when the origin errors or times out.
	Stale bool `json:"stale,omitempty"`
	// Refresh fetches a new copy in the background while visitors get the saved page.
	Refresh bool `json:"refresh,omitempty"`
	// Honor keeps Cache-Control and Expires from the application. The lifetime is then only a fallback.
	Honor bool `json:"honor,omitempty"`
	// Cookies are name fragments. A request carrying one is neither stored nor served from cache.
	Cookies string `json:"cookies,omitempty"`
	// Skip is a list of path prefixes that are never stored, such as /wp-admin.
	Skip string `json:"skip,omitempty"`
}

func (c *Cache) UnmarshalJSON(data []byte) error {
	var raw struct {
		On      *bool  `json:"on"`
		Enabled *bool  `json:"enabled"`
		Valid   string `json:"valid"`
		Path    string `json:"path"`
		Preset  string `json:"preset"`
		Size    string `json:"size"`
		Idle    string `json:"idle"`
		Stale   bool   `json:"stale"`
		Refresh bool   `json:"refresh"`
		Honor   bool   `json:"honor"`
		Cookies string `json:"cookies"`
		Skip    string `json:"skip"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.Valid = raw.Valid
	c.Path = raw.Path
	c.Preset = raw.Preset
	c.Size = raw.Size
	c.Idle = raw.Idle
	c.Stale = raw.Stale
	c.Refresh = raw.Refresh
	c.Honor = raw.Honor
	c.Cookies = raw.Cookies
	c.Skip = raw.Skip
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

var cacheSizes = map[string]string{
	"":     "",
	"256m": "256m",
	"1g":   "",
	"5g":   "5g",
	"10g":  "10g",
	"20g":  "20g",
}

var cacheIdles = map[string]string{
	"":    "",
	"1h":  "1h",
	"1d":  "",
	"7d":  "7d",
	"30d": "30d",
}

var cachePresets = map[string]string{
	"":          "",
	"public":    "",
	"wordpress": "wordpress",
	"app":       "app",
	"long":      "long",
	"custom":    "custom",
}

var cookieNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
var skipPathRe = regexp.MustCompile(`^/[A-Za-z0-9_./-]+$`)

func normalizeSiteCache(in *Cache) (*Cache, error) {
	if in == nil {
		return nil, nil
	}
	valid, ok := cacheValids[strings.TrimSpace(in.Valid)]
	if !ok {
		return nil, fmt.Errorf("cache time must be 10m, 30m, 1h, 6h, 12h, 1d, or 7d")
	}
	size, ok := cacheSizes[strings.TrimSpace(in.Size)]
	if !ok {
		return nil, fmt.Errorf("cache disk size must be 256m, 1g, 5g, 10g, or 20g")
	}
	idle, ok := cacheIdles[strings.TrimSpace(in.Idle)]
	if !ok {
		return nil, fmt.Errorf("cache idle time must be 1h, 1d, 7d, or 30d")
	}
	preset, ok := cachePresets[strings.TrimSpace(in.Preset)]
	if !ok {
		return nil, fmt.Errorf("unknown cache preset")
	}
	cookies, err := normalizeCacheCookies(in.Cookies)
	if err != nil {
		return nil, err
	}
	skip, err := normalizeCacheSkip(in.Skip)
	if err != nil {
		return nil, err
	}
	pth, err := normalizeCachePath(in.Path)
	if err != nil {
		return nil, err
	}
	out := &Cache{
		On: in.On, Valid: valid, Path: pth, Preset: preset, Size: size, Idle: idle,
		Stale: in.Stale, Refresh: in.Refresh, Honor: in.Honor, Cookies: cookies, Skip: skip,
	}
	if !out.On && out.Path == "" && out.Valid == "" && out.Preset == "" && out.Size == "" && out.Idle == "" && !out.Stale && !out.Refresh && !out.Honor && out.Cookies == "" && out.Skip == "" {
		return nil, nil
	}
	return out, nil
}

func normalizeCacheCookies(raw string) (string, error) {
	parts := splitCacheList(raw)
	if len(parts) > 12 {
		return "", fmt.Errorf("cache cookies are limited to 12 names")
	}
	seen := map[string]bool{}
	var out []string
	for _, part := range parts {
		if !cookieNameRe.MatchString(part) {
			return "", fmt.Errorf("cache cookie %q is not a cookie name", part)
		}
		if seen[part] {
			continue
		}
		seen[part] = true
		out = append(out, part)
	}
	return strings.Join(out, " "), nil
}

func normalizeCacheSkip(raw string) (string, error) {
	parts := splitCacheList(raw)
	if len(parts) > 12 {
		return "", fmt.Errorf("cache skip paths are limited to 12")
	}
	seen := map[string]bool{}
	var out []string
	for _, part := range parts {
		if !strings.HasPrefix(part, "/") {
			part = "/" + part
		}
		part = strings.TrimRight(part, "/")
		if part == "" || strings.Contains(part, "..") || !skipPathRe.MatchString(part) {
			return "", fmt.Errorf("cache skip %q must be a path such as /wp-admin", part)
		}
		if seen[part] {
			continue
		}
		seen[part] = true
		out = append(out, part)
	}
	return strings.Join(out, " "), nil
}

func splitCacheList(raw string) []string {
	raw = strings.NewReplacer(",", " ", "\n", " ").Replace(raw)
	var out []string
	for _, part := range strings.Fields(raw) {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
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

// CachePathLine is the http-level proxy_cache_path for one site directory.
func CachePathLine(dir, zone string, c Cache) string {
	size := c.Size
	if size == "" {
		size = "1g"
	}
	idle := c.Idle
	if idle == "" {
		idle = "1d"
	}
	return fmt.Sprintf("proxy_cache_path %s levels=1:2 keys_zone=%s:10m max_size=%s inactive=%s use_temp_path=off;\n", dir, zone, size, idle)
}

// CacheDirectives are the nginx lines placed inside a proxy location.
func CacheDirectives(zone string, c Cache) string {
	if zone == "" {
		return ""
	}
	valid := c.Valid
	if valid == "" {
		valid = "1h"
	}
	var b strings.Builder
	if c.Cookies != "" || c.Skip != "" {
		b.WriteString("        set $siroc_cache_skip 0;\n")
		if pattern := skipRegex(c.Skip); pattern != "" {
			fmt.Fprintf(&b, "        if ($request_uri ~* \"%s\") { set $siroc_cache_skip 1; }\n", pattern)
		}
		if pattern := cookieRegex(c.Cookies); pattern != "" {
			fmt.Fprintf(&b, "        if ($http_cookie ~* \"%s\") { set $siroc_cache_skip 1; }\n", pattern)
		}
	}
	fmt.Fprintf(&b, "        proxy_cache %s;\n", zone)
	b.WriteString("        proxy_cache_key $scheme$host$request_uri;\n")
	fmt.Fprintf(&b, "        proxy_cache_valid 200 301 302 %s;\n", valid)
	b.WriteString("        proxy_cache_lock on;\n")
	if c.Stale || c.Refresh {
		stale := "error timeout http_500 http_502 http_503 http_504"
		if c.Refresh {
			stale = "updating " + stale
		}
		fmt.Fprintf(&b, "        proxy_cache_use_stale %s;\n", stale)
	}
	if c.Refresh {
		b.WriteString("        proxy_cache_background_update on;\n")
	}
	if c.Honor {
		b.WriteString("        proxy_ignore_headers Set-Cookie;\n")
	} else {
		b.WriteString("        proxy_ignore_headers Set-Cookie Cache-Control Expires X-Accel-Expires;\n")
	}
	bypass := "$http_authorization"
	if c.Cookies != "" || c.Skip != "" {
		bypass = "$siroc_cache_skip $http_authorization"
	}
	fmt.Fprintf(&b, "        proxy_cache_bypass %s;\n", bypass)
	fmt.Fprintf(&b, "        proxy_no_cache %s;\n", bypass)
	b.WriteString("        add_header X-Cache-Status $upstream_cache_status always;\n")
	return b.String()
}

func skipRegex(raw string) string {
	var parts []string
	for _, path := range strings.Fields(raw) {
		parts = append(parts, regexp.QuoteMeta(path)+`(/|$)`)
	}
	if len(parts) == 0 {
		return ""
	}
	return "^(" + strings.Join(parts, "|") + ")"
}

func cookieRegex(raw string) string {
	var parts []string
	for _, name := range strings.Fields(raw) {
		parts = append(parts, regexp.QuoteMeta(name))
	}
	return strings.Join(parts, "|")
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

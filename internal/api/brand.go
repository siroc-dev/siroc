package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/siroc-dev/siroc/internal/rpc"
)

const (
	settingThemeStyle  = "panel.theme.style"
	settingThemeColor  = "panel.theme.color"
	settingThemeCustom = "panel.theme.custom"
	settingLogo        = "panel.logo"
	settingFavicon     = "panel.favicon"
	settingBrandRev    = "panel.brand.rev"
	settingMonitor     = "panel.service.monitor"
	brandUploadMax     = 2 << 20
)

var (
	themeStyles = map[string]bool{"auto": true, "light": true, "dark": true}
	themeColors = map[string]bool{
		"default": true, "mint": true, "violet": true, "sky": true,
		"sakura": true, "blackgold": true, "custom": true,
	}
	hexColorRE = regexp.MustCompile(`^#([0-9a-fA-F]{6})$`)
	logoTypes  = map[string]string{
		".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
		".svg": "image/svg+xml", ".webp": "image/webp", ".gif": "image/gif",
	}
	faviconTypes = map[string]string{
		".ico": "image/x-icon", ".png": "image/png", ".svg": "image/svg+xml",
		".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".webp": "image/webp",
	}
	serviceManage = map[string]string{
		"nginx":      "/nginx",
		"apache":     "/apache",
		"php":        "/php-fpm",
		"mysql":      "/mysql",
		"mariadb":    "/mariadb",
		"waf":        "/security",
		"fail2ban":   "/tools",
		"ufw":        "/security",
		"openssh":    "/tools",
		"vsftpd":     "/software",
		"redis":      "/redis",
		"docker":     "/software",
		"memcached":  "/tools",
		"supervisor": "/software",
		"clamav":     "/security",
		"openvas":    "/security",
	}
)

type brandPublic struct {
	ThemeStyle  string `json:"themeStyle"`
	ThemeColor  string `json:"themeColor"`
	ThemeCustom string `json:"themeCustom,omitempty"`
	LogoURL     string `json:"logoUrl,omitempty"`
	FaviconURL  string `json:"faviconUrl,omitempty"`
	HasLogo     bool   `json:"hasLogo"`
	HasFavicon  bool   `json:"hasFavicon"`
}

type panelService struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Version string `json:"version"`
	Active  bool   `json:"active"`
	Monitor bool   `json:"monitor"`
	Manage  string `json:"manage,omitempty"`
}

func normalizeThemeStyle(v string) (string, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return "auto", nil
	}
	if !themeStyles[v] {
		return "", fmt.Errorf("theme style must be auto, light, or dark")
	}
	return v, nil
}

func normalizeThemeColor(v string) (string, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return "default", nil
	}
	if !themeColors[v] {
		return "", fmt.Errorf("unknown theme color")
	}
	return v, nil
}

func normalizeThemeCustom(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", nil
	}
	if !hexColorRE.MatchString(v) {
		return "", fmt.Errorf("custom color must be #RRGGBB")
	}
	return strings.ToLower(v), nil
}

func (s *Server) brandDir() string {
	return filepath.Join(s.Cfg.DataDir, "brand")
}

func (s *Server) settingOr(key, fallback string) string {
	v, err := s.Store.Setting(key)
	if err != nil || strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func (s *Server) brandRev() string {
	return s.settingOr(settingBrandRev, "0")
}

func (s *Server) bumpBrandRev() {
	_ = s.Store.SetSetting(settingBrandRev, strconv.FormatInt(time.Now().UnixNano(), 10))
}

func (s *Server) brandFile(kind string) (abs, ctype string, ok bool) {
	key := settingLogo
	types := logoTypes
	if kind == "favicon" {
		key = settingFavicon
		types = faviconTypes
	}
	name, err := s.Store.Setting(key)
	if err != nil || name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return "", "", false
	}
	ext := strings.ToLower(filepath.Ext(name))
	ctype = types[ext]
	if ctype == "" {
		return "", "", false
	}
	abs = filepath.Join(s.brandDir(), filepath.Base(name))
	if _, err := os.Stat(abs); err != nil {
		return "", "", false
	}
	return abs, ctype, true
}

func (s *Server) publicBrand() brandPublic {
	style, _ := normalizeThemeStyle(s.settingOr(settingThemeStyle, "auto"))
	color, _ := normalizeThemeColor(s.settingOr(settingThemeColor, "default"))
	custom, _ := normalizeThemeCustom(s.settingOr(settingThemeCustom, ""))
	out := brandPublic{ThemeStyle: style, ThemeColor: color, ThemeCustom: custom}
	rev := s.brandRev()
	if _, _, ok := s.brandFile("logo"); ok {
		out.HasLogo = true
		out.LogoURL = "/brand/logo?v=" + rev
	}
	if _, _, ok := s.brandFile("favicon"); ok {
		out.HasFavicon = true
		out.FaviconURL = "/brand/favicon?v=" + rev
	}
	return out
}

func (s *Server) getBrand(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.publicBrand())
}

func (s *Server) serveBrandFile(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		abs, ctype, ok := s.brandFile(kind)
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", ctype)
		w.Header().Set("Cache-Control", "public, max-age=3600")
		http.ServeFile(w, r, abs)
	}
}

func (s *Server) putPanelSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ThemeStyle  string `json:"themeStyle"`
		ThemeColor  string `json:"themeColor"`
		ThemeCustom string `json:"themeCustom"`
	}
	if !decode(w, r, &body) {
		return
	}
	style, err := normalizeThemeStyle(body.ThemeStyle)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	color, err := normalizeThemeColor(body.ThemeColor)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	custom, err := normalizeThemeCustom(body.ThemeCustom)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if color == "custom" && custom == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("custom color is required"))
		return
	}
	if err := s.Store.SetSetting(settingThemeStyle, style); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.Store.SetSetting(settingThemeColor, color); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.Store.SetSetting(settingThemeCustom, custom); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, s.publicBrand())
}

func (s *Server) uploadBrand(w http.ResponseWriter, r *http.Request) {
	kind := chi.URLParam(r, "kind")
	if kind != "logo" && kind != "favicon" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("kind must be logo or favicon"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, brandUploadMax+512)
	if err := r.ParseMultipartForm(brandUploadMax); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("file too large (max 2 MB)"))
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("file is required"))
		return
	}
	defer file.Close()
	ext := strings.ToLower(filepath.Ext(hdr.Filename))
	types := logoTypes
	key := settingLogo
	if kind == "favicon" {
		types = faviconTypes
		key = settingFavicon
	}
	if types[ext] == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("unsupported image type"))
		return
	}
	if err := os.MkdirAll(s.brandDir(), 0750); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	name := kind + ext
	abs := filepath.Join(s.brandDir(), name)
	tmp, err := os.CreateTemp(s.brandDir(), kind+".*")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	tmpName := tmp.Name()
	_, copyErr := io.Copy(tmp, io.LimitReader(file, brandUploadMax+1))
	closeErr := tmp.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(tmpName)
		writeErr(w, http.StatusBadRequest, fmt.Errorf("could not save file"))
		return
	}
	if err := os.Rename(tmpName, abs); err != nil {
		_ = os.Remove(tmpName)
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	s.removeBrandFilesExcept(kind, name)
	if err := s.Store.SetSetting(key, name); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	s.bumpBrandRev()
	writeJSON(w, http.StatusOK, s.publicBrand())
}

func (s *Server) deleteBrand(w http.ResponseWriter, r *http.Request) {
	kind := chi.URLParam(r, "kind")
	if kind != "logo" && kind != "favicon" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("kind must be logo or favicon"))
		return
	}
	s.removeBrandFiles(kind)
	key := settingLogo
	if kind == "favicon" {
		key = settingFavicon
	}
	if err := s.Store.SetSetting(key, ""); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	s.bumpBrandRev()
	writeJSON(w, http.StatusOK, s.publicBrand())
}

func (s *Server) removeBrandFiles(kind string) {
	s.removeBrandFilesExcept(kind, "")
}

func (s *Server) removeBrandFilesExcept(kind, keep string) {
	entries, err := os.ReadDir(s.brandDir())
	if err != nil {
		return
	}
	prefix := kind + "."
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) && e.Name() != keep {
			_ = os.Remove(filepath.Join(s.brandDir(), e.Name()))
		}
	}
}

func (s *Server) monitorMap() map[string]bool {
	out := map[string]bool{}
	if s.Store == nil {
		return out
	}
	raw, err := s.Store.Setting(settingMonitor)
	if err != nil || strings.TrimSpace(raw) == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func (s *Server) monitored(mon map[string]bool, name string) bool {
	v, ok := mon[name]
	if !ok {
		return true
	}
	return v
}

func (s *Server) listPanelServices(w http.ResponseWriter, _ *http.Request) {
	list, err := s.Agent.Packages()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, s.buildPanelServices(list))
}

func (s *Server) buildPanelServices(list []rpc.PackageInfo) []panelService {
	mon := s.monitorMap()
	var out []panelService
	for _, p := range list {
		if !p.Installed {
			continue
		}
		if p.Name == "php" {
			for _, ver := range p.InstalledVersions {
				name := "php" + ver
				active := p.VersionActive[ver]
				out = append(out, panelService{
					Name:    name,
					Title:   "PHP " + ver + " FPM",
					Version: ver,
					Active:  active,
					Monitor: s.monitored(mon, name),
					Manage:  "/php-fpm",
				})
			}
			continue
		}
		if p.Service == "" {
			continue
		}
		out = append(out, panelService{
			Name:    p.Name,
			Title:   p.Title,
			Version: p.Version,
			Active:  p.Active,
			Monitor: s.monitored(mon, p.Name),
			Manage:  serviceManage[p.Name],
		})
	}
	if out == nil {
		out = []panelService{}
	}
	return out
}

func (s *Server) putServiceMonitor(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string `json:"name"`
		Monitor bool   `json:"monitor"`
	}
	if !decode(w, r, &body) {
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" || strings.ContainsAny(name, "/\\ \t") {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid service name"))
		return
	}
	mon := s.monitorMap()
	mon[name] = body.Monitor
	raw, err := json.Marshal(mon)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.Store.SetSetting(settingMonitor, string(raw)); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": name, "monitor": body.Monitor})
}

package hosting

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func TestWPStaffRole(t *testing.T) {
	if !wpStaffRole("administrator") || !wpStaffRole("editor") || !wpStaffRole("author, editor") {
		t.Fatal("admin and editor should be included")
	}
	if wpStaffRole("subscriber") || wpStaffRole("customer") || wpStaffRole("author") || wpStaffRole("contributor") || wpStaffRole("") {
		t.Fatal("regular roles should be excluded")
	}
}

func TestWPUserCacheHour(t *testing.T) {
	now := time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)
	if !wpUserCacheFresh(now.Add(-59*time.Minute), now) {
		t.Fatal("cache younger than an hour should be used")
	}
	if wpUserCacheFresh(now.Add(-time.Hour), now) {
		t.Fatal("cache at one hour should be refreshed")
	}
	dir := t.TempDir()
	prev := wpUserCacheDir
	wpUserCacheDir = dir
	t.Cleanup(func() { wpUserCacheDir = prev })
	users := []rpc.WPUser{{ID: 1, Login: "editor1", Roles: "editor"}}
	if err := saveWPUserCache("novellucky", "/home/novellucky/web", now, users); err != nil {
		t.Fatal(err)
	}
	got, ok := loadWPUserCache("novellucky", "/home/novellucky/web", now.Add(30*time.Minute))
	if !ok || len(got.Users) != 1 || got.Users[0].Login != "editor1" {
		t.Fatalf("cache miss: %+v ok=%v", got, ok)
	}
	if _, ok := loadWPUserCache("novellucky", "/home/novellucky/web", now.Add(time.Hour)); ok {
		t.Fatal("expired cache should miss")
	}
	dropWPUserCache("novellucky", "/home/novellucky/web")
	if _, err := filepath.Glob(filepath.Join(dir, "*.json")); err != nil {
		t.Fatal(err)
	}
}

func TestWPSiteCacheHour(t *testing.T) {
	now := time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	prev := wpSiteCacheDir
	wpSiteCacheDir = dir
	t.Cleanup(func() { wpSiteCacheDir = prev })
	err := saveWPSiteCache("novellucky", "/home/novellucky/web", wpSiteCacheFile{
		At:      now,
		Name:    "Novel Lucky",
		Version: "6.8",
		Plugins: []rpc.WPPlugin{{Name: "akismet", Status: "active"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := loadWPSiteCache("novellucky", "/home/novellucky/web", now.Add(20*time.Minute))
	if !ok || got.Name != "Novel Lucky" || len(got.Plugins) != 1 {
		t.Fatalf("site cache miss: %+v ok=%v", got, ok)
	}
	st := got.status()
	if st.CachedAt == "" || st.Plugins[0].Name != "akismet" {
		t.Fatalf("status: %+v", st)
	}
	if _, ok := loadWPSiteCache("novellucky", "/home/novellucky/web", now.Add(time.Hour)); ok {
		t.Fatal("expired site cache should miss")
	}
}

package api

import (
	"testing"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func TestNormalizeTheme(t *testing.T) {
	style, err := normalizeThemeStyle("Dark")
	if err != nil || style != "dark" {
		t.Fatalf("style=%q err=%v", style, err)
	}
	if _, err := normalizeThemeStyle("neon"); err == nil {
		t.Fatal("expected invalid style")
	}
	color, err := normalizeThemeColor("Sky")
	if err != nil || color != "sky" {
		t.Fatalf("color=%q err=%v", color, err)
	}
	if _, err := normalizeThemeColor("orange"); err == nil {
		t.Fatal("expected invalid color")
	}
	hex, err := normalizeThemeCustom("#AaBbCc")
	if err != nil || hex != "#aabbcc" {
		t.Fatalf("hex=%q err=%v", hex, err)
	}
	if _, err := normalizeThemeCustom("blue"); err == nil {
		t.Fatal("expected invalid hex")
	}
}

func TestBuildPanelServicesExpandsPHP(t *testing.T) {
	s := &Server{}
	rows := s.buildPanelServices([]rpc.PackageInfo{
		{Name: "nginx", Title: "Nginx", Installed: true, Version: "1.24", Service: "nginx", Active: true},
		{Name: "git", Title: "Git", Installed: true, Version: "2.40"},
		{
			Name:              "php",
			Title:             "PHP",
			Installed:         true,
			Service:           "php-fpm",
			InstalledVersions: []string{"8.3", "8.4"},
			VersionActive:     map[string]bool{"8.3": true, "8.4": false},
		},
	})
	if len(rows) != 3 {
		t.Fatalf("rows=%d %#v", len(rows), rows)
	}
	if rows[0].Name != "nginx" || !rows[0].Monitor {
		t.Fatalf("nginx row %#v", rows[0])
	}
	if rows[1].Name != "php8.3" || rows[1].Title != "PHP 8.3 FPM" || !rows[1].Active || rows[1].Manage != "/php-fpm" {
		t.Fatalf("php 8.3 %#v", rows[1])
	}
	if rows[2].Name != "php8.4" || rows[2].Active {
		t.Fatalf("php 8.4 %#v", rows[2])
	}
}

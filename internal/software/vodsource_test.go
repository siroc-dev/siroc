package software

import "testing"

func TestVodModuleSource(t *testing.T) {
	url := vodModuleArchive()
	if !stringsContains(url, "github.com/frank-kios/nginx-vod-module/archive/cf4da9ba2a639e2b51146450e0458e5d5857922c.tar.gz") {
		t.Fatalf("archive: %s", url)
	}
	if vodModuleStamp() != "frank-kios@cf4da9ba2a63" {
		t.Fatalf("stamp: %s", vodModuleStamp())
	}
}

func TestStampBaseIgnoresVodChoice(t *testing.T) {
	old := "1.26.0 vod=1.33 lua=v0.10.26 ndk=v0.3.3 cc=-O2"
	off := "1.26.0 vod=off lua=v0.10.26 ndk=v0.3.3 cc=-O2"
	on := "1.26.0 vod=" + vodModuleStamp() + " lua=v0.10.26 ndk=v0.3.3 cc=-O2"
	if stampBase(old) != stampBase(off) || stampBase(off) != stampBase(on) {
		t.Fatalf("base changed: %q %q %q", stampBase(old), stampBase(off), stampBase(on))
	}
	if stampVod(old) != "1.33" || stampVod(on) != vodModuleStamp() || stampVod(off) != "off" {
		t.Fatalf("vod token: %q %q %q", stampVod(old), stampVod(on), stampVod(off))
	}
}

func TestParseVodChoice(t *testing.T) {
	if !parseVodChoice("on", false) || parseVodChoice("off", true) {
		t.Fatal("explicit choice")
	}
	if !parseVodChoice("", true) || parseVodChoice("", false) {
		t.Fatal("missing choice follows the installed module")
	}
}

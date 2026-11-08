package software

import "strings"

const (
	vodModuleOwner  = "frank-kios"
	vodModuleRepo   = "nginx-vod-module"
	vodModuleCommit = "cf4da9ba2a639e2b51146450e0458e5d5857922c"
	vodChoiceOn     = "on"
	vodChoiceOff    = "off"
)

func vodModuleArchive() string {
	return "https://github.com/" + vodModuleOwner + "/" + vodModuleRepo + "/archive/" + vodModuleCommit + ".tar.gz"
}

func vodModuleStamp() string {
	return vodModuleOwner + "@" + vodModuleCommit[:12]
}

// stampBase drops the vod token so turning the module off does not rebuild Lua.
func stampBase(stamp string) string {
	const key = " vod="
	i := strings.Index(stamp, key)
	if i < 0 {
		return stamp
	}
	rest := stamp[i+len(key):]
	if j := strings.Index(rest, " "); j >= 0 {
		return stamp[:i] + stamp[i+len(key)+j:]
	}
	return strings.TrimSpace(stamp[:i])
}

func stampVod(stamp string) string {
	const key = " vod="
	i := strings.Index(stamp, key)
	if i < 0 {
		return ""
	}
	rest := stamp[i+len(key):]
	if j := strings.Index(rest, " "); j >= 0 {
		return rest[:j]
	}
	return rest
}

// parseVodChoice reads the saved install choice. An empty choice keeps a module
// that is already on disk, so an older server does not lose VOD on upgrade.
func parseVodChoice(raw string, moduleExists bool) bool {
	switch strings.TrimSpace(raw) {
	case vodChoiceOn:
		return true
	case vodChoiceOff:
		return false
	default:
		return moduleExists
	}
}

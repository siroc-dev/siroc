package backup

import (
	"fmt"
	"path"
	"strings"
	"time"
)

// WeekdayDir is the folder used by the 7-day layout. Names are lowercase English.
func WeekdayDir(t time.Time) string {
	switch t.Weekday() {
	case time.Monday:
		return "monday"
	case time.Tuesday:
		return "tuesday"
	case time.Wednesday:
		return "wednesday"
	case time.Thursday:
		return "thursday"
	case time.Friday:
		return "friday"
	case time.Saturday:
		return "saturday"
	default:
		return "sunday"
	}
}

// NormalizeLayout accepts the default layout or the 7-day weekday folders.
func NormalizeLayout(v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "keep":
		return "", nil
	case "week":
		return "week", nil
	default:
		return "", fmt.Errorf("keep must be one folder or 7 weekday folders")
	}
}

// WeekRemote is the destination folder and file name for one archive.
// The 7-day layout uses a stable file name so the next backup on that weekday replaces it.
func WeekRemote(layout string, now time.Time, username, archiveBase string) (folder, name string) {
	if layout != "week" {
		return "", archiveBase
	}
	return WeekdayDir(now), username + ".tar.gz"
}

// DurableLocal is the only directory and file name kept for a local archive.
// An empty base uses /var/backups/siroc/<username>. The 7-day layout adds the weekday folder.
func DurableLocal(base, layout string, now time.Time, username, stampName string) (dir, name string) {
	base = strings.TrimSpace(strings.ReplaceAll(base, "\\", "/"))
	if base == "" {
		base = "/var/backups/siroc/" + username
	}
	base = path.Clean(base)
	folder, file := WeekRemote(layout, now, username, stampName)
	if folder != "" {
		return path.Join(base, folder), file
	}
	return base, file
}

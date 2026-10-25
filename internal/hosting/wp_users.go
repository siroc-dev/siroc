package hosting

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/layout"
	"github.com/siroc-dev/siroc/internal/rpc"
)

const wpUserCacheTTL = time.Hour

var wpUserCacheDir = filepath.Join(layout.Cache, "wp-users")

type wpUserCacheFile struct {
	At    time.Time    `json:"at"`
	Users []rpc.WPUser `json:"users"`
}

func wpStaffRole(roles string) bool {
	for _, role := range strings.FieldsFunc(roles, func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == ' '
	}) {
		switch strings.ToLower(role) {
		case "administrator", "editor":
			return true
		}
	}
	return false
}

func wpUserCacheFresh(at, now time.Time) bool {
	if at.IsZero() || now.Before(at) {
		return false
	}
	return now.Sub(at) < wpUserCacheTTL
}

func wpCacheKey(user, root string) string {
	sum := sha256.Sum256([]byte(user + "\n" + filepath.Clean(root)))
	return hex.EncodeToString(sum[:16])
}

func wpUserCachePath(user, root string) string {
	return filepath.Join(wpUserCacheDir, wpCacheKey(user, root)+".json")
}

func readWPUserCache(user, root string) (wpUserCacheFile, bool) {
	b, err := os.ReadFile(wpUserCachePath(user, root))
	if err != nil {
		return wpUserCacheFile{}, false
	}
	var c wpUserCacheFile
	if json.Unmarshal(b, &c) != nil || c.At.IsZero() {
		return wpUserCacheFile{}, false
	}
	if c.Users == nil {
		c.Users = []rpc.WPUser{}
	}
	return c, true
}

func loadWPUserCache(user, root string, now time.Time) (wpUserCacheFile, bool) {
	c, ok := readWPUserCache(user, root)
	if !ok || !wpUserCacheFresh(c.At, now) {
		return wpUserCacheFile{}, false
	}
	return c, true
}

func saveWPUserCache(user, root string, at time.Time, users []rpc.WPUser) error {
	if users == nil {
		users = []rpc.WPUser{}
	}
	if err := os.MkdirAll(wpUserCacheDir, 0700); err != nil {
		return err
	}
	b, err := json.Marshal(wpUserCacheFile{At: at.UTC(), Users: users})
	if err != nil {
		return err
	}
	return os.WriteFile(wpUserCachePath(user, root), b, 0600)
}

func dropWPUserCache(user, root string) {
	_ = os.Remove(wpUserCachePath(user, root))
}

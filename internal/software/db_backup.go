package software

import (
	"sort"
	"strings"
	"time"
)

const preUpgradeDumpDir = "/var/backups/siroc/db-upgrade"

// pickMySQLDataDir prefers the Ubuntu 26.04 directory when ibdata1 is there.
// MariaDB.org packages default to /var/lib/mysql and then refuse to start.
func pickMySQLDataDir(found []string) string {
	order := []string{"/var/lib/mariadb", "/var/lib/mysql", "/siroc/db/mysql"}
	have := map[string]bool{}
	for _, dir := range found {
		have[dir] = true
	}
	for _, dir := range order {
		if have[dir] {
			return dir
		}
	}
	return ""
}

func preUpgradeDumpName(engine string, now time.Time) string {
	if engine != "mysql" && engine != "mariadb" {
		engine = "mysql"
	}
	return engine + "-" + now.UTC().Format("20060102-150405") + ".sql.gz"
}

// dumpsToRemove returns older dump file names, keeping the newest keep files.
func dumpsToRemove(names []string, keep int) []string {
	if keep < 1 {
		keep = 1
	}
	var dumps []string
	for _, name := range names {
		if strings.HasSuffix(name, ".sql.gz") {
			dumps = append(dumps, name)
		}
	}
	sort.Strings(dumps)
	if len(dumps) <= keep {
		return nil
	}
	return append([]string(nil), dumps[:len(dumps)-keep]...)
}

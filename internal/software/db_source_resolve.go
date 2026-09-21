package software

import (
	"encoding/json"
	"strings"

	"github.com/siroc-dev/siroc/internal/version"
)

var mariadbSourceFallback = map[string]string{
	"10.11": "10.11.14",
	"11.4":  "11.4.13",
	"11.8":  "11.8.3",
}

var mysqlSourceFallback = map[string]string{
	"8.0": "8.0.43",
	"8.4": "8.4.6",
}

func latestSeriesVersion(series string, ids []string) string {
	best := ""
	prefix := series + "."
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != series && !strings.HasPrefix(id, prefix) {
			continue
		}
		if best == "" || version.Compare(best, id) < 0 {
			best = id
		}
	}
	return best
}

func parseMariaDBLatest(body []byte, series string) string {
	var root struct {
		Releases map[string]json.RawMessage `json:"releases"`
	}
	if json.Unmarshal(body, &root) != nil {
		return ""
	}
	ids := make([]string, 0, len(root.Releases))
	for id := range root.Releases {
		ids = append(ids, id)
	}
	return latestSeriesVersion(series, ids)
}

func parseMySQLTagLatest(body []byte, series string) string {
	var tags []struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(body, &tags) != nil {
		return ""
	}
	prefix := "mysql-" + series + "."
	var ids []string
	for _, t := range tags {
		if strings.HasPrefix(t.Name, prefix) {
			ids = append(ids, strings.TrimPrefix(t.Name, "mysql-"))
		}
	}
	return latestSeriesVersion(series, ids)
}

func mariadbSourceURL(full string) string {
	return "https://archive.mariadb.org/mariadb-" + full + "/source/mariadb-" + full + ".tar.gz"
}

func mysqlSourceURL(full string) string {
	series := full
	if i := strings.LastIndex(full, "."); i > 0 {
		series = full[:i]
	}
	return "https://cdn.mysql.com/Downloads/MySQL-" + series + "/mysql-boost-" + full + ".tar.gz"
}

func mysqlSourceURLAlt(full string) string {
	return "https://github.com/mysql/mysql-server/archive/refs/tags/mysql-" + full + ".tar.gz"
}

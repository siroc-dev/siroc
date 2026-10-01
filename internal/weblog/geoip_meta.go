package weblog

import "time"

func geoipMonth(t time.Time) string {
	return t.UTC().Format("2006-01")
}

func dbipLiteURL(kind, month string) string {
	return "https://download.db-ip.com/free/dbip-" + kind + "-lite-" + month + ".mmdb.gz"
}

func geoipMonths(now time.Time) []string {
	t := now.UTC()
	return []string{geoipMonth(t), geoipMonth(t.AddDate(0, -1, 0))}
}

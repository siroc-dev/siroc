//go:build linux

package weblog

import (
	"net"
	"os"
	"sync"

	"github.com/oschwald/maxminddb-golang"
)

var (
	geoMu  sync.Mutex
	cityDB *maxminddb.Reader
	asnDB  *maxminddb.Reader
)

type cityRec struct {
	Country struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"country"`
	City struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"city"`
}

type asnRec struct {
	Number uint   `maxminddb:"autonomous_system_number"`
	Org    string `maxminddb:"autonomous_system_organization"`
}

// LookupPlace reads the local DB-IP city and ASN databases.
func LookupPlace(ip string) Place {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return Place{}
	}
	if parsed.IsPrivate() || parsed.IsLoopback() || parsed.IsLinkLocalUnicast() {
		return Place{Location: "Private network"}
	}
	if _, err := os.Stat(geoipDir + "/city.mmdb"); err != nil {
		_ = EnsureGeoIP()
	}
	geoMu.Lock()
	defer geoMu.Unlock()
	openGeo()
	var city cityRec
	var asn asnRec
	if cityDB != nil {
		_ = cityDB.Lookup(parsed, &city)
	}
	if asnDB != nil {
		_ = asnDB.Lookup(parsed, &asn)
	}
	return formatPlace(city.City.Names["en"], city.Country.Names["en"], asn.Number, asn.Org)
}

func openGeo() {
	if cityDB == nil {
		if r, err := maxminddb.Open(geoipDir + "/city.mmdb"); err == nil {
			cityDB = r
		}
	}
	if asnDB == nil {
		if r, err := maxminddb.Open(geoipDir + "/asn.mmdb"); err == nil {
			asnDB = r
		}
	}
}

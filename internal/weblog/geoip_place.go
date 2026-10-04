package weblog

import "fmt"

// Place is a short GeoIP answer for a banned address.
type Place struct {
	ASN      string
	ASOrg    string
	Country  string
	City     string
	Location string
}

func formatPlace(city, country string, asn uint, org string) Place {
	p := Place{Country: country, City: city, ASOrg: org}
	switch {
	case city != "" && country != "":
		p.Location = city + ", " + country
	case country != "":
		p.Location = country
	case city != "":
		p.Location = city
	}
	if asn > 0 {
		p.ASN = fmt.Sprintf("AS%d", asn)
	}
	return p
}

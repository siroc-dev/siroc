package dbmgmt

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

const (
	AccessLocal  = "local"
	AccessAll    = "all"
	AccessIPs    = "ips"
	maxAccessIPs = 20
)

// LocalHosts are the socket login and the TCP login phpMyAdmin uses.
func LocalHosts() []string {
	return []string{"localhost", "127.0.0.1"}
}

// ResolveAccess turns a panel choice into the stored value and the MySQL accounts to keep.
// Localhost and 127.0.0.1 are always included so the website and phpMyAdmin keep working.
func ResolveAccess(access string, hosts []string) (stored string, mysql []string, err error) {
	access = strings.ToLower(strings.TrimSpace(access))
	switch access {
	case "", AccessLocal:
		return "", LocalHosts(), nil
	case AccessAll, "%":
		return "%", append(LocalHosts(), "%"), nil
	case AccessIPs:
	default:
		return "", nil, fmt.Errorf("access must be local, all, or ips")
	}
	seen := map[string]struct{}{}
	var ips []string
	for _, raw := range hosts {
		for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
			return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' '
		}) {
			ip, err := cleanAccessIP(part)
			if err != nil {
				return "", nil, err
			}
			if _, ok := seen[ip]; ok {
				continue
			}
			seen[ip] = struct{}{}
			ips = append(ips, ip)
		}
	}
	if len(ips) == 0 {
		return "", nil, fmt.Errorf("enter at least one IP address")
	}
	if len(ips) > maxAccessIPs {
		return "", nil, fmt.Errorf("at most %d IP addresses", maxAccessIPs)
	}
	sort.Strings(ips)
	return strings.Join(ips, ","), append(LocalHosts(), ips...), nil
}

// DescribeStored splits the database column into the panel choice.
func DescribeStored(stored string) (access string, hosts []string) {
	stored = strings.TrimSpace(stored)
	switch stored {
	case "", "local":
		return AccessLocal, nil
	case "%", "all":
		return AccessAll, nil
	}
	var ips []string
	for _, part := range strings.Split(stored, ",") {
		ip, err := cleanAccessIP(part)
		if err != nil {
			continue
		}
		ips = append(ips, ip)
	}
	if len(ips) == 0 {
		return AccessLocal, nil
	}
	return AccessIPs, ips
}

// ExpandStored is the MySQL host list for a stored access value.
func ExpandStored(stored string) []string {
	access, hosts := DescribeStored(stored)
	_, mysql, err := ResolveAccess(access, hosts)
	if err != nil {
		return LocalHosts()
	}
	return mysql
}

// HostsToDrop are accounts that should be removed when access changes.
// Localhost and 127.0.0.1 are never dropped.
func HostsToDrop(oldHosts, newHosts []string) []string {
	keep := map[string]struct{}{}
	for _, host := range newHosts {
		keep[host] = struct{}{}
	}
	var drop []string
	seen := map[string]struct{}{}
	for _, host := range oldHosts {
		if host == "localhost" || host == "127.0.0.1" {
			continue
		}
		if _, ok := keep[host]; ok {
			continue
		}
		if _, ok := seen[host]; ok {
			continue
		}
		seen[host] = struct{}{}
		drop = append(drop, host)
	}
	return drop
}

func cleanAccessIP(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("invalid IP address")
	}
	if raw == "%" || strings.EqualFold(raw, "localhost") {
		return "", fmt.Errorf("%s is not an IP address", raw)
	}
	ip := net.ParseIP(raw)
	if ip == nil {
		return "", fmt.Errorf("invalid IP address %s", raw)
	}
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsMulticast() {
		return "", fmt.Errorf("%s is already covered by localhost", ip.String())
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String(), nil
	}
	return ip.String(), nil
}

// AccessSQL creates or updates one database user on each host and grants its databases.
func AccessSQL(user, password string, dbNames, hosts []string, canCreate bool) []string {
	var parts []string
	for _, host := range hosts {
		parts = append(parts,
			fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%s' IDENTIFIED BY '%s';", sqlQuote(user), sqlQuote(host), sqlQuote(password)),
			fmt.Sprintf("ALTER USER '%s'@'%s' IDENTIFIED BY '%s';", sqlQuote(user), sqlQuote(host), sqlQuote(password)),
		)
		for _, db := range dbNames {
			if strings.TrimSpace(db) == "" {
				continue
			}
			parts = append(parts, GrantDatabaseSQL(user, host, db))
		}
		if canCreate {
			parts = append(parts, CreatePrivSQL(user, host, true))
		}
	}
	parts = append(parts, "FLUSH PRIVILEGES;")
	return parts
}

func DropUserSQL(user, host string) string {
	return fmt.Sprintf("DROP USER IF EXISTS '%s'@'%s';", sqlQuote(user), sqlQuote(host))
}

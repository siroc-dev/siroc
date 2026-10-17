package dbmgmt

import (
	"fmt"
	"sort"
	"strings"
)

// CreatePrivSQL grants or removes the right to create a database with any name.
func CreatePrivSQL(user, host string, on bool) string {
	if on {
		return fmt.Sprintf("GRANT CREATE ON *.* TO '%s'@'%s';", sqlQuote(user), host)
	}
	return fmt.Sprintf("REVOKE CREATE ON *.* FROM '%s'@'%s';", sqlQuote(user), host)
}

// PrefixRevokeSQL removes an older grant that allowed every account_name database.
func PrefixRevokeSQL(user, account, host string) string {
	pattern := strings.NewReplacer(`\`, `\\`, "_", `\_`, "%", `\%`, "`", "``").Replace(account) + `\_%`
	return fmt.Sprintf("REVOKE ALL PRIVILEGES ON `%s`.* FROM '%s'@'%s';", pattern, sqlQuote(user), host)
}

func GrantDatabaseSQL(user, host, db string) string {
	return fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'%s';", strings.ReplaceAll(db, "`", "``"), sqlQuote(user), host)
}

func sqlQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return s
}

type SyncAccount struct {
	Username string
	Users    []string
}

type SyncDB struct {
	Username string
	DBUser   string
	DBName   string
}

type SchemaGrant struct {
	DBName string
	DBUser string
}

// LiteralDBName returns a privilege-table database name that is one exact schema.
// A % pattern is not a single database.
func LiteralDBName(raw string) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\\' && i+1 < len(raw) {
			b.WriteByte(raw[i+1])
			i++
			continue
		}
		if raw[i] == '%' {
			return "", false
		}
		b.WriteByte(raw[i])
	}
	name := strings.TrimSpace(b.String())
	if name == "" || systemSchema(name) {
		return "", false
	}
	return name, true
}

// SyncCreated lists databases a permitted user can use, and databases that user created
// which still need a grant. Names are not filtered by account prefix.
func SyncCreated(schemas []string, grants []SchemaGrant, creators map[string]string, accounts []SyncAccount) (have []SyncDB, grant []SyncDB) {
	allowed := map[string]string{}
	for _, acc := range accounts {
		for _, user := range acc.Users {
			user = strings.TrimSpace(user)
			if user == "" {
				continue
			}
			if _, ok := allowed[user]; !ok {
				allowed[user] = strings.TrimSpace(acc.Username)
			}
		}
	}
	present := map[string]struct{}{}
	for _, schema := range schemas {
		schema = strings.TrimSpace(schema)
		if schema == "" || systemSchema(schema) {
			continue
		}
		present[schema] = struct{}{}
	}
	owner := map[string]string{}
	taken := map[string]struct{}{}
	for _, g := range grants {
		if _, ok := present[g.DBName]; !ok {
			continue
		}
		if _, ok := allowed[g.DBUser]; ok {
			if _, seen := owner[g.DBName]; !seen {
				owner[g.DBName] = g.DBUser
			}
			continue
		}
		if g.DBUser != "" {
			taken[g.DBName] = struct{}{}
		}
	}
	for schema, user := range owner {
		have = append(have, SyncDB{Username: allowed[user], DBUser: user, DBName: schema})
	}
	sort.Slice(have, func(i, j int) bool { return have[i].DBName < have[j].DBName })
	seenGrant := map[string]struct{}{}
	for schema, user := range creators {
		schema = strings.TrimSpace(schema)
		user = mysqlAccount(user)
		acc, ok := allowed[user]
		if !ok {
			continue
		}
		if _, ok := present[schema]; !ok || systemSchema(schema) {
			continue
		}
		if _, ok := owner[schema]; ok {
			continue
		}
		if _, ok := taken[schema]; ok {
			continue
		}
		if _, ok := seenGrant[schema]; ok {
			continue
		}
		seenGrant[schema] = struct{}{}
		grant = append(grant, SyncDB{Username: acc, DBUser: user, DBName: schema})
	}
	sort.Slice(grant, func(i, j int) bool { return grant[i].DBName < grant[j].DBName })
	return have, grant
}

func ParseGrantRows(raw string) []SchemaGrant {
	var out []SchemaGrant
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		name, ok := LiteralDBName(parts[0])
		if !ok {
			continue
		}
		user := strings.TrimSpace(parts[1])
		if user == "" {
			continue
		}
		out = append(out, SchemaGrant{DBName: name, DBUser: user})
	}
	return out
}

func ParseCreatorRows(raw string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		name, ok := CreatedDatabase(parts[1])
		if !ok {
			continue
		}
		user := mysqlAccount(parts[0])
		if user == "" {
			continue
		}
		out[name] = user
	}
	return out
}

func CreatorsFromAudit(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name, ok := CreatedDatabase(line)
		if !ok {
			continue
		}
		user := auditUser(line)
		if user == "" {
			continue
		}
		out[name] = user
	}
	return out
}

func CreatedDatabase(sql string) (string, bool) {
	s := strings.TrimSpace(sql)
	s = strings.Trim(s, ";")
	s = strings.TrimSpace(s)
	low := strings.ToLower(s)
	idx := strings.Index(low, "create database")
	kind := len("create database")
	if idx < 0 {
		idx = strings.Index(low, "create schema")
		kind = len("create schema")
	}
	if idx < 0 {
		return "", false
	}
	rest := strings.TrimSpace(s[idx+kind:])
	fields := strings.Fields(rest)
	if len(fields) >= 3 && strings.EqualFold(fields[0], "if") && strings.EqualFold(fields[1], "not") && strings.EqualFold(fields[2], "exists") {
		fields = fields[3:]
	}
	if len(fields) == 0 {
		return "", false
	}
	name := trimIdent(fields[0])
	if name == "" || systemSchema(name) {
		return "", false
	}
	return name, true
}

func trimIdent(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if s[0] == '`' {
		if i := strings.Index(s[1:], "`"); i >= 0 {
			return s[1 : 1+i]
		}
	}
	if s[0] == '\'' || s[0] == '"' {
		if i := strings.IndexByte(s[1:], s[0]); i >= 0 {
			return s[1 : 1+i]
		}
	}
	end := 0
	for end < len(s) {
		c := s[end]
		if c == '_' || (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			end++
			continue
		}
		break
	}
	return s[:end]
}

func mysqlAccount(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if i := strings.Index(raw, "["); i >= 0 {
		if j := strings.Index(raw[i:], "]"); j > 1 {
			return strings.TrimSpace(raw[i+1 : i+j])
		}
	}
	if i := strings.Index(raw, "@"); i >= 0 {
		raw = raw[:i]
	}
	return strings.Trim(raw, " `")
}

func auditUser(line string) string {
	parts := strings.SplitN(line, ",", 9)
	if len(parts) < 3 {
		return ""
	}
	return mysqlAccount(strings.TrimSpace(parts[2]))
}

func systemSchema(name string) bool {
	switch strings.ToLower(name) {
	case "information_schema", "mysql", "performance_schema", "sys":
		return true
	default:
		return false
	}
}

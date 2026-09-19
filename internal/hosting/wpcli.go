//go:build linux

package hosting

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

var (
	wpPrefixRe    = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{1,14}_$`)
	wpCLITokRe    = regexp.MustCompile(`^[a-z][a-z0-9-]{0,40}$`)
	wpCLIFlagRe   = regexp.MustCompile(`^--?[A-Za-z][A-Za-z0-9_-]*$`)
	wpCLIArgRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:\\/@+,-]*$`)
	wpTableNameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)
)

func randomWPPrefix() string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	const alnum = letters + "0123456789"
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("w%04x_", time.Now().UnixNano()%0xffff)
	}
	out := make([]byte, 6)
	out[0] = letters[int(b[0])%len(letters)]
	for i := 1; i < 5; i++ {
		out[i] = alnum[int(b[i])%len(alnum)]
	}
	out[5] = '_'
	return string(out)
}

func validWPPrefix(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return randomWPPrefix(), nil
	}
	if !strings.HasSuffix(p, "_") {
		p += "_"
	}
	if !wpPrefixRe.MatchString(p) {
		return "", fmt.Errorf("table prefix must start with a letter, use A-Z 0-9 _, and end with _")
	}
	return p, nil
}

func wpCLIScript(root, raw string) (string, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "wp ")
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return "", fmt.Errorf("wp-cli command required")
	}
	var cmd []string
	i := 0
	for i < len(fields) && i < 4 && wpCLITokRe.MatchString(fields[i]) {
		cmd = append(cmd, fields[i])
		i++
	}
	if len(cmd) == 0 {
		return "", fmt.Errorf("invalid wp-cli command")
	}
	if err := blockedWPCLI(cmd, fields[i:]); err != nil {
		return "", err
	}
	args := []string{"wp", "--path=" + shellQuote(root), "--no-color"}
	for _, c := range cmd {
		args = append(args, shellQuote(c))
	}
	for _, f := range fields[i:] {
		switch f {
		case "--no-color", "-q", "--quiet":
			continue
		}
		if strings.HasPrefix(f, "-") {
			flag, val, cut := strings.Cut(f, "=")
			if !wpCLIFlagRe.MatchString(flag) {
				return "", fmt.Errorf("flag %s is not allowed", f)
			}
			if cut && val != "" && !wpCLIArgRe.MatchString(val) {
				return "", fmt.Errorf("flag value %q is not allowed", val)
			}
			args = append(args, shellQuote(f))
			continue
		}
		if !wpCLIArgRe.MatchString(f) {
			return "", fmt.Errorf("argument %q is not allowed", f)
		}
		args = append(args, shellQuote(f))
	}
	return strings.Join(args, " "), nil
}

func blockedWPCLI(cmd, rest []string) error {
	head := cmd[0]
	switch head {
	case "eval", "eval-file", "shell", "server":
		return fmt.Errorf("%s cannot run from the panel", head)
	case "db":
		if len(cmd) > 1 {
			switch cmd[1] {
			case "query", "cli", "drop", "reset", "clean", "create":
				return fmt.Errorf("wp db %s cannot run from the panel", cmd[1])
			}
		}
	case "config":
		if len(cmd) > 1 && (cmd[1] == "create" || cmd[1] == "delete" || cmd[1] == "shuffle-salts") {
			return fmt.Errorf("wp config %s cannot run from the panel", cmd[1])
		}
		if len(cmd) > 1 && cmd[1] == "set" {
			for _, a := range append(append([]string{}, cmd[2:]...), rest...) {
				if strings.EqualFold(strings.Trim(a, `"'`), "table_prefix") {
					return fmt.Errorf("change the table prefix from the Site tab")
				}
			}
		}
	case "core":
		if len(cmd) > 1 && (cmd[1] == "download" || cmd[1] == "install" || cmd[1] == "multisite-install" || cmd[1] == "multisite-convert") {
			return fmt.Errorf("wp core %s cannot run from the panel", cmd[1])
		}
	}
	_ = rest
	return nil
}

func listWPCLI(user, root string) []rpc.WPCmd {
	raw, err := wpRun(user, root, "cli", "cmd-dump")
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	if i := strings.Index(raw, "{"); i > 0 {
		raw = raw[i:]
	}
	var rootCmd wpDump
	if json.Unmarshal([]byte(raw), &rootCmd) != nil {
		return nil
	}
	var out []rpc.WPCmd
	flattenWPDump("", rootCmd, &out)
	if len(out) > 160 {
		out = out[:160]
	}
	return out
}

type wpDump struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Subcommands []wpDump `json:"subcommands"`
}

func flattenWPDump(parent string, node wpDump, out *[]rpc.WPCmd) {
	name := strings.TrimSpace(node.Name)
	full := name
	if parent != "" && parent != "wp" {
		full = parent + " " + name
	} else if parent == "wp" {
		full = name
	}
	if len(node.Subcommands) == 0 {
		if full == "" || full == "wp" {
			return
		}
		if blockedWPCLI(strings.Fields(full), nil) != nil {
			return
		}
		*out = append(*out, rpc.WPCmd{Name: full, Description: node.Description})
		return
	}
	nextParent := full
	if parent == "" {
		nextParent = "wp"
	}
	for _, sub := range node.Subcommands {
		flattenWPDump(nextParent, sub, out)
	}
}

func changeWPPrefix(user, root, next string) (string, error) {
	next, err := validWPPrefix(next)
	if err != nil {
		return "", err
	}
	old := strings.TrimSpace(mustWP(user, root, "config", "get", "table_prefix", "--type=variable"))
	if old == "" {
		old = strings.TrimSpace(mustWP(user, root, "db", "prefix"))
	}
	if old == "" || strings.HasPrefix(strings.ToLower(old), "error:") {
		return "", fmt.Errorf("could not read the current table prefix")
	}
	if old == next {
		return "table prefix is already " + next, nil
	}
	tables, msg, err := listWPTables(user, root)
	if err != nil {
		return msg, fmt.Errorf("list tables: %w", err)
	}
	var pairs []string
	taken := map[string]struct{}{}
	for _, t := range tables {
		taken[t] = struct{}{}
	}
	for _, t := range tables {
		if !strings.HasPrefix(t, old) || !wpTableNameRe.MatchString(t) {
			continue
		}
		neu := next + strings.TrimPrefix(t, old)
		if !wpTableNameRe.MatchString(neu) {
			return "", fmt.Errorf("invalid new table name %q", neu)
		}
		if _, ok := taken[neu]; ok {
			return "", fmt.Errorf("table %s already exists", neu)
		}
		oldID, err := sqlIdent(t)
		if err != nil {
			return "", err
		}
		newID, err := sqlIdent(neu)
		if err != nil {
			return "", err
		}
		pairs = append(pairs, oldID+" TO "+newID)
		taken[neu] = struct{}{}
	}
	var logs []string
	if len(pairs) > 0 {
		sql := "RENAME TABLE " + strings.Join(pairs, ", ")
		if msg, err := mysqlExec(user, root, sql); err != nil {
			return msg, fmt.Errorf("rename tables: %w", err)
		}
		logs = append(logs, fmt.Sprintf("renamed %d tables", len(pairs)))
	}
	if msg, err := rewritePrefixedColumn(user, root, next+"options", "option_name", old, next); err != nil {
		return msg, err
	}
	if msg, err := rewritePrefixedColumn(user, root, next+"usermeta", "meta_key", old, next); err != nil {
		return msg, err
	}
	_, _ = rewritePrefixedColumn(user, root, next+"sitemeta", "meta_key", old, next)
	if err := setWPConfigPrefix(root, user, next); err != nil {
		return "", fmt.Errorf("update wp-config.php: %w", err)
	}
	_, _ = wpRun(user, root, "cache", "flush")
	logs = append(logs, "wp-config.php table_prefix is now "+next)
	return strings.Join(logs, "\n"), nil
}

func listWPTables(user, root string) ([]string, string, error) {
	raw, err := wpRun(user, root, "db", "tables", "--all-tables-with-prefix")
	if err != nil {
		raw, err = mysqlExec(user, root, "SHOW TABLES")
		if err != nil {
			return nil, raw, err
		}
	}
	var tables []string
	for _, t := range strings.Split(raw, "\n") {
		t = strings.TrimSpace(t)
		if t == "" || strings.HasPrefix(t, "Tables_in_") {
			continue
		}
		if wpTableNameRe.MatchString(t) {
			tables = append(tables, t)
		}
	}
	return tables, raw, nil
}

func rewritePrefixedColumn(user, root, table, column, old, next string) (string, error) {
	if !wpTableNameRe.MatchString(table) || !wpTableNameRe.MatchString(column) {
		return "", fmt.Errorf("invalid identifier")
	}
	tbl, err := sqlIdent(table)
	if err != nil {
		return "", err
	}
	col, err := sqlIdent(column)
	if err != nil {
		return "", err
	}
	like := sqlLikePrefix(old)
	sql := fmt.Sprintf(
		"UPDATE %s SET %s = CONCAT(%s, SUBSTRING(%s, CHAR_LENGTH(%s)+1)) WHERE %s LIKE %s ESCAPE '|'",
		tbl, col, sqlString(next), col, sqlString(old), col, sqlString(like),
	)
	msg, err := mysqlExec(user, root, sql)
	if err != nil {
		low := strings.ToLower(msg + " " + err.Error())
		if strings.Contains(low, "doesn't exist") || strings.Contains(low, "unknown table") {
			return "", nil
		}
		return msg, fmt.Errorf("update %s.%s: %w", table, column, err)
	}
	return msg, nil
}

var (
	wpDBIdentRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)
	wpDBHostRe  = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	wpPrefixLineRe = regexp.MustCompile(`(?m)^\$table_prefix\s*=\s*['"].*?['"];`)
)

func mysqlExec(user, root, sql string) (string, error) {
	name := strings.TrimSpace(mustWP(user, root, "config", "get", "DB_NAME"))
	dbUser := strings.TrimSpace(mustWP(user, root, "config", "get", "DB_USER"))
	pass := mustWP(user, root, "config", "get", "DB_PASSWORD")
	host := strings.TrimSpace(mustWP(user, root, "config", "get", "DB_HOST"))
	if host == "" {
		host = "localhost"
	}
	if i := strings.IndexByte(host, ':'); i > 0 {
		host = host[:i]
	}
	if !wpDBIdentRe.MatchString(name) || !wpDBIdentRe.MatchString(dbUser) || !wpDBHostRe.MatchString(host) {
		return "", fmt.Errorf("invalid database connection settings")
	}
	if strings.ContainsAny(pass, "\x00\n\r") {
		return "", fmt.Errorf("invalid database password")
	}
	script := "mariadb --skip-ssl -h " + shellQuote(host) + " -u " + shellQuote(dbUser) + " -p" + shellQuote(pass) + " " + shellQuote(name) + " -N -e " + shellQuote(sql)
	return runAs(user, root, 2*time.Minute, script)
}

func setWPConfigPrefix(root, user, prefix string) error {
	path := filepath.Join(root, "wp-config.php")
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	loc := wpPrefixLineRe.FindIndex(b)
	if loc == nil {
		return fmt.Errorf("wp-config.php has no $table_prefix")
	}
	neu := []byte("$table_prefix = '" + prefix + "';")
	out := append([]byte{}, b[:loc[0]]...)
	out = append(out, neu...)
	out = append(out, b[loc[1]:]...)
	if err := os.WriteFile(path, out, 0644); err != nil {
		return err
	}
	own := user + ":" + user
	_ = exec.Command("chown", own, path).Run()
	return nil
}

func sqlIdent(name string) (string, error) {
	if !wpTableNameRe.MatchString(name) {
		return "", fmt.Errorf("invalid SQL identifier")
	}
	return "`" + name + "`", nil
}

func sqlString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func sqlLikePrefix(p string) string {
	p = strings.ReplaceAll(p, "|", "||")
	p = strings.ReplaceAll(p, "%", "|%")
	p = strings.ReplaceAll(p, "_", "|_")
	return p + "%"
}

func mustWP(user, root string, args ...string) string {
	out, _ := wpRun(user, root, args...)
	return out
}

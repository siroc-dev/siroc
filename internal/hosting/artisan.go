//go:build linux

package hosting

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

var (
	artisanCmdRe      = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9:_-]{0,79}$`)
	artisanFlagNameRe = regexp.MustCompile(`^--?[A-Za-z][A-Za-z0-9_-]*$`)
	artisanArgRe      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:\\/@+-]*$`)
)

func artisanScript(php, raw string) (string, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "php ")
	s = strings.TrimPrefix(s, "artisan ")
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return "", fmt.Errorf("artisan command required")
	}
	name := fields[0]
	if !artisanCmdRe.MatchString(name) {
		return "", fmt.Errorf("invalid artisan command")
	}
	switch name {
	case "tinker", "serve", "dump-server", "pail", "docs", "completion":
		return "", fmt.Errorf("%s cannot run from the panel", name)
	}
	args := []string{shellQuote(php), "artisan", shellQuote(name)}
	for _, f := range fields[1:] {
		switch f {
		case "--no-interaction", "-n", "--no-ansi", "-q", "--quiet", "-v", "-vv", "-vvv", "--ansi":
			continue
		}
		if strings.HasPrefix(f, "-") {
			flag, val, cut := strings.Cut(f, "=")
			if !artisanFlagNameRe.MatchString(flag) {
				return "", fmt.Errorf("flag %s is not allowed", f)
			}
			if cut && !artisanArgRe.MatchString(val) {
				return "", fmt.Errorf("flag value %q is not allowed", val)
			}
			args = append(args, shellQuote(f))
			continue
		}
		if !artisanArgRe.MatchString(f) {
			return "", fmt.Errorf("argument %q is not allowed", f)
		}
		args = append(args, shellQuote(f))
	}
	args = append(args, "--no-interaction", "--no-ansi")
	return strings.Join(args, " "), nil
}

func listArtisan(user, root, phpVer string) []rpc.ArtisanCmd {
	if !fileOK(filepath.Join(root, "artisan")) {
		return nil
	}
	raw, err := runAs(user, root, 45*time.Second, shellQuote(phpBin(phpVer))+" artisan list --format=json --no-interaction --no-ansi")
	if err != nil || raw == "" {
		return nil
	}
	if i := strings.Index(raw, "{"); i > 0 {
		raw = raw[i:]
	}
	var parsed struct {
		Commands []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Hidden      bool   `json:"hidden"`
		} `json:"commands"`
	}
	if json.Unmarshal([]byte(raw), &parsed) != nil {
		return nil
	}
	out := make([]rpc.ArtisanCmd, 0, len(parsed.Commands))
	seen := map[string]struct{}{}
	for _, c := range parsed.Commands {
		name := strings.TrimSpace(c.Name)
		if c.Hidden || name == "" || strings.Contains(name, " ") || !artisanCmdRe.MatchString(name) {
			continue
		}
		switch name {
		case "tinker", "serve", "dump-server", "pail", "docs", "completion", "list", "help":
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, rpc.ArtisanCmd{Name: name, Description: c.Description})
	}
	return out
}

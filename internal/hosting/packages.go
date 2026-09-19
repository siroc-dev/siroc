//go:build linux

package hosting

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

var (
	composerPkgRe = regexp.MustCompile(`(?i)^[a-z0-9]([_.-]?[a-z0-9]+)*/[a-z0-9]([_.-]?[a-z0-9]+)*$`)
	npmPkgRe      = regexp.MustCompile(`^(?:@[a-z0-9][a-z0-9._~-]*/)?[a-z0-9][a-z0-9._~-]*$`)
)

func fillPackages(st *rpc.LaravelStatus, user, root, phpVer string) {
	if st == nil {
		return
	}
	var wg sync.WaitGroup
	var composer []rpc.PkgRow
	var npm []rpc.PkgRow
	var scripts []string
	var artisan []rpc.ArtisanCmd
	if st.HasComposer {
		wg.Add(1)
		go func() {
			defer wg.Done()
			composer = listComposerPkgs(user, root)
		}()
	}
	if st.HasPackage {
		wg.Add(1)
		go func() {
			defer wg.Done()
			npm, scripts = listNPMPkgs(user, root)
		}()
	}
	if st.HasArtisan {
		wg.Add(1)
		go func() {
			defer wg.Done()
			artisan = listArtisan(user, root, phpVer)
		}()
	}
	wg.Wait()
	st.ComposerPkgs = composer
	st.NPMPkgs = npm
	st.NPMScripts = scripts
	st.ArtisanCmds = artisan
}

func listComposerPkgs(user, root string) []rpc.PkgRow {
	raw, err := os.ReadFile(filepath.Join(root, "composer.json"))
	if err != nil {
		return nil
	}
	var file struct {
		Require    map[string]string `json:"require"`
		RequireDev map[string]string `json:"require-dev"`
	}
	if json.Unmarshal(raw, &file) != nil {
		return nil
	}
	byName := map[string]*rpc.PkgRow{}
	add := func(section string, m map[string]string) {
		for name, constraint := range m {
			if name == "php" || strings.HasPrefix(name, "ext-") {
				continue
			}
			byName[name] = &rpc.PkgRow{Name: name, Section: section, Constraint: constraint}
		}
	}
	add("require", file.Require)
	add("require-dev", file.RequireDev)

	show, _ := runAs(user, root, 45*time.Second, "composer show --direct --format=json --no-interaction --no-ansi 2>/dev/null")
	var shown struct {
		Installed []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"installed"`
	}
	if json.Unmarshal([]byte(show), &shown) == nil {
		for _, p := range shown.Installed {
			row := byName[p.Name]
			if row == nil {
				row = &rpc.PkgRow{Name: p.Name, Section: "require"}
				byName[p.Name] = row
			}
			row.Installed = strings.TrimPrefix(p.Version, "v")
		}
	}

	outJSON, _ := runAs(user, root, 90*time.Second, "composer outdated --direct --format=json --no-interaction --no-ansi 2>/dev/null")
	var outdated struct {
		Installed []struct {
			Name         string `json:"name"`
			Version      string `json:"version"`
			Latest       string `json:"latest"`
			LatestStatus string `json:"latest-status"`
		} `json:"installed"`
	}
	if json.Unmarshal([]byte(outJSON), &outdated) == nil {
		for _, p := range outdated.Installed {
			row := byName[p.Name]
			if row == nil {
				row = &rpc.PkgRow{Name: p.Name, Section: "require"}
				byName[p.Name] = row
			}
			if row.Installed == "" {
				row.Installed = strings.TrimPrefix(p.Version, "v")
			}
			row.Latest = strings.TrimPrefix(p.Latest, "v")
			row.Update = composerUpdateKind(p.LatestStatus, row.Installed, row.Latest)
		}
	}

	out := make([]rpc.PkgRow, 0, len(byName))
	for _, row := range byName {
		if row.Latest == "" {
			row.Latest = row.Installed
		}
		if row.Update == "" {
			row.Update = composerUpdateKind("", row.Installed, row.Latest)
		}
		out = append(out, *row)
	}
	sortPkgRows(out)
	return out
}

func composerUpdateKind(status, installed, latest string) string {
	if latest == "" || installed == "" || installed == latest {
		return ""
	}
	switch status {
	case "semver-safe-update":
		return "minor"
	case "update-possible":
		return "major"
	}
	return "available"
}

func listNPMPkgs(user, root string) ([]rpc.PkgRow, []string) {
	raw, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return nil, nil
	}
	var file struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
		Scripts         map[string]string `json:"scripts"`
	}
	if json.Unmarshal(raw, &file) != nil {
		return nil, nil
	}
	byName := map[string]*rpc.PkgRow{}
	add := func(section string, m map[string]string) {
		for name, constraint := range m {
			byName[name] = &rpc.PkgRow{Name: name, Section: section, Constraint: constraint}
		}
	}
	add("dependencies", file.Dependencies)
	add("devDependencies", file.DevDependencies)

	var scripts []string
	for name := range file.Scripts {
		scripts = append(scripts, name)
	}

	fillNPMInstalled(root, byName)

	if fileOK(filepath.Join(root, "node_modules")) {
		lsOut, _ := runAs(user, root, 20*time.Second, shellQuote(resolveUserBin(user, "npm"))+" ls --depth=0 --json --omit=peer")
		var tree struct {
			Dependencies map[string]struct {
				Version string `json:"version"`
			} `json:"dependencies"`
		}
		if json.Unmarshal(jsonObject(lsOut), &tree) == nil {
			for name, p := range tree.Dependencies {
				row := byName[name]
				if row == nil || p.Version == "" {
					continue
				}
				row.Installed = p.Version
			}
		}

		outdated, _ := runAs(user, root, 25*time.Second, shellQuote(resolveUserBin(user, "npm"))+" outdated --json")
		var npmOut map[string]struct {
			Current string `json:"current"`
			Wanted  string `json:"wanted"`
			Latest  string `json:"latest"`
			Type    string `json:"type"`
		}
		if json.Unmarshal(jsonObject(outdated), &npmOut) == nil {
			for name, p := range npmOut {
				row := byName[name]
				if row == nil {
					row = &rpc.PkgRow{Name: name, Section: p.Type}
					byName[name] = row
				}
				if row.Installed == "" {
					row.Installed = p.Current
				}
				row.Wanted = p.Wanted
				row.Latest = p.Latest
				row.Update = bumpKind(row.Installed, p.Wanted, p.Latest)
			}
		}
	}

	var missing []string
	for name, row := range byName {
		if row.Latest == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		for name, ver := range npmRegistryLatest(missing) {
			row := byName[name]
			if row == nil || ver == "" {
				continue
			}
			row.Latest = ver
			if row.Update == "" {
				row.Update = bumpKind(row.Installed, "", ver)
			}
		}
	}

	out := make([]rpc.PkgRow, 0, len(byName))
	for _, row := range byName {
		if row.Latest == "" {
			row.Latest = row.Installed
		}
		if row.Update == "" {
			row.Update = bumpKind(row.Installed, row.Wanted, row.Latest)
		}
		out = append(out, *row)
	}
	sortPkgRows(out)
	sort.Strings(scripts)
	return out, scripts
}

func jsonObject(s string) []byte {
	i := strings.Index(s, "{")
	j := strings.LastIndex(s, "}")
	if i < 0 || j <= i {
		return nil
	}
	return []byte(s[i : j+1])
}

func fillNPMInstalled(root string, byName map[string]*rpc.PkgRow) {
	for name, row := range byName {
		p := filepath.Join(root, "node_modules", filepath.FromSlash(name), "package.json")
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var pkg struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(b, &pkg) == nil && pkg.Version != "" {
			row.Installed = pkg.Version
		}
	}
	raw, err := os.ReadFile(filepath.Join(root, "package-lock.json"))
	if err != nil {
		raw, err = os.ReadFile(filepath.Join(root, "npm-shrinkwrap.json"))
	}
	if err != nil {
		return
	}
	var lock struct {
		Packages map[string]struct {
			Version string `json:"version"`
		} `json:"packages"`
		Dependencies map[string]struct {
			Version string `json:"version"`
		} `json:"dependencies"`
	}
	if json.Unmarshal(raw, &lock) != nil {
		return
	}
	for key, p := range lock.Packages {
		name := lockPackageName(key)
		row := byName[name]
		if row == nil || row.Installed != "" || p.Version == "" {
			continue
		}
		row.Installed = p.Version
	}
	for name, p := range lock.Dependencies {
		row := byName[name]
		if row == nil || row.Installed != "" || p.Version == "" {
			continue
		}
		row.Installed = p.Version
	}
}

func lockPackageName(key string) string {
	key = strings.TrimPrefix(strings.ReplaceAll(key, "\\", "/"), "./")
	if key == "" || !strings.HasPrefix(key, "node_modules/") {
		return ""
	}
	rest := strings.TrimPrefix(key, "node_modules/")
	if strings.Contains(rest, "/node_modules/") {
		return ""
	}
	if strings.HasPrefix(rest, "@") {
		parts := strings.Split(rest, "/")
		if len(parts) < 2 {
			return ""
		}
		return parts[0] + "/" + parts[1]
	}
	if i := strings.Index(rest, "/"); i >= 0 {
		return rest[:i]
	}
	return rest
}

func bumpKind(installed, wanted, latest string) string {
	target := latest
	if wanted != "" {
		target = wanted
	}
	if latest != "" && installed != "" && latest != installed {
		if wanted != "" && wanted != latest && wanted != installed {
			if greaterVersion(latest, installed) {
				return "major"
			}
		}
		if wanted != "" && wanted != installed {
			if greaterVersion(wanted, installed) {
				return "minor"
			}
		}
		if greaterVersion(latest, installed) {
			majI, _, _ := verParts(installed)
			majL, _, _ := verParts(latest)
			if majL > majI {
				return "major"
			}
			return "minor"
		}
	}
	if target == "" || installed == "" || target == installed {
		return ""
	}
	if greaterVersion(target, installed) {
		return "available"
	}
	return ""
}

func verParts(s string) (maj, min, pat int) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	n := func(i int) int {
		if i >= len(parts) {
			return 0
		}
		v := 0
		for _, c := range parts[i] {
			if c < '0' || c > '9' {
				break
			}
			v = v*10 + int(c-'0')
		}
		return v
	}
	return n(0), n(1), n(2)
}

func greaterVersion(a, b string) bool {
	am, ai, ap := verParts(a)
	bm, bi, bp := verParts(b)
	if am != bm {
		return am > bm
	}
	if ai != bi {
		return ai > bi
	}
	return ap > bp
}

func npmRegistryLatest(names []string) map[string]string {
	out := map[string]string{}
	if len(names) == 0 {
		return out
	}
	if len(names) > 40 {
		names = names[:40]
	}
	client := &http.Client{Timeout: 6 * time.Second}
	var mu sync.Mutex
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, name := range names {
		name := name
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			enc := strings.ReplaceAll(url.PathEscape(name), "%40", "@")
			b, err := httpGet(client, "https://registry.npmjs.org/"+enc+"/latest")
			if err != nil {
				return
			}
			var pkg struct {
				Version string `json:"version"`
			}
			if json.Unmarshal(b, &pkg) != nil || pkg.Version == "" {
				return
			}
			mu.Lock()
			out[name] = pkg.Version
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out
}

func sortPkgRows(rows []rpc.PkgRow) {
	sort.Slice(rows, func(i, j int) bool {
		iu, ju := rows[i].Update != "", rows[j].Update != ""
		if iu != ju {
			return iu
		}
		return rows[i].Name < rows[j].Name
	})
}

func pkgSearch(kind, q string) ([]rpc.PkgHit, error) {
	q = strings.TrimSpace(q)
	if len(q) < 2 || len(q) > 80 {
		return nil, fmt.Errorf("search needs 2-80 characters")
	}
	client := &http.Client{Timeout: 8 * time.Second}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "npm":
		u := "https://registry.npmjs.org/-/v1/search?text=" + url.QueryEscape(q) + "&size=8"
		b, err := httpGet(client, u)
		if err != nil {
			return nil, err
		}
		var res struct {
			Objects []struct {
				Package struct {
					Name        string `json:"name"`
					Description string `json:"description"`
					Version     string `json:"version"`
				} `json:"package"`
			} `json:"objects"`
		}
		if json.Unmarshal(b, &res) != nil {
			return nil, fmt.Errorf("npm search failed")
		}
		var hits []rpc.PkgHit
		for _, o := range res.Objects {
			hits = append(hits, rpc.PkgHit{Name: o.Package.Name, Description: o.Package.Description, Version: o.Package.Version})
		}
		return hits, nil
	default:
		u := "https://packagist.org/search.json?q=" + url.QueryEscape(q) + "&per_page=8"
		b, err := httpGet(client, u)
		if err != nil {
			return nil, err
		}
		var res struct {
			Results []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"results"`
		}
		if json.Unmarshal(b, &res) != nil {
			return nil, fmt.Errorf("packagist search failed")
		}
		var hits []rpc.PkgHit
		for _, o := range res.Results {
			hits = append(hits, rpc.PkgHit{Name: o.Name, Description: o.Description})
		}
		return hits, nil
	}
}

func httpGet(client *http.Client, raw string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "siroc")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 256<<10))
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("search HTTP %d", res.StatusCode)
	}
	return b, nil
}

func composerScript(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "composer ")
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return "composer install --no-interaction --prefer-dist", nil
	}
	dev := false
	var args []string
	for _, f := range fields[1:] {
		switch f {
		case "--dev", "-dev":
			dev = true
		case "--no-dev", "--no-interaction", "--prefer-dist", "-n", "-W", "--with-all-dependencies":
		default:
			if strings.HasPrefix(f, "-") {
				return "", fmt.Errorf("flag %s is not allowed", f)
			}
			args = append(args, f)
		}
	}
	switch fields[0] {
	case "install":
		if len(args) > 0 {
			return "", fmt.Errorf("use require to add a package")
		}
		return "composer install --no-interaction --prefer-dist", nil
	case "update":
		if len(args) == 0 {
			return "composer update --no-interaction --prefer-dist", nil
		}
		pkg, _ := splitComposerArg(args[0])
		if err := validComposerPkg(pkg); err != nil {
			return "", err
		}
		return "composer update --no-interaction --prefer-dist --with-all-dependencies " + shellQuote(pkg), nil
	case "dump", "dump-autoload", "dumpautoload":
		return "composer dump-autoload -o --no-interaction", nil
	case "require":
		if len(args) == 0 {
			return "", fmt.Errorf("package name required")
		}
		pkg, ver := splitComposerArg(args[0])
		if err := validComposerPkg(pkg); err != nil {
			return "", err
		}
		spec := pkg
		if ver != "" {
			if err := validConstraint(ver); err != nil {
				return "", err
			}
			spec += ":" + ver
		}
		cmd := "composer require --no-interaction --prefer-dist " + shellQuote(spec)
		if dev {
			cmd += " --dev"
		}
		return cmd, nil
	case "remove", "rm":
		if len(args) == 0 {
			return "", fmt.Errorf("package name required")
		}
		pkg, _ := splitComposerArg(args[0])
		if err := validComposerPkg(pkg); err != nil {
			return "", err
		}
		return "composer remove --no-interaction " + shellQuote(pkg), nil
	default:
		if strings.Contains(fields[0], "/") {
			pkg, ver := splitComposerArg(fields[0])
			if err := validComposerPkg(pkg); err != nil {
				return "", err
			}
			spec := pkg
			if ver != "" {
				if err := validConstraint(ver); err != nil {
					return "", err
				}
				spec += ":" + ver
			}
			return "composer require --no-interaction --prefer-dist " + shellQuote(spec), nil
		}
		return "", fmt.Errorf("allowed: install, update, require, remove, dump-autoload")
	}
}

func npmScript(raw string, scripts []string) (string, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "npm ")
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return "npm install", nil
	}
	dev := false
	var args []string
	for _, f := range fields[1:] {
		switch f {
		case "--save-dev", "-D", "--dev":
			dev = true
		case "--save", "-P", "--no-fund", "--no-audit":
		default:
			if strings.HasPrefix(f, "-") {
				return "", fmt.Errorf("flag %s is not allowed", f)
			}
			args = append(args, f)
		}
	}
	switch fields[0] {
	case "install", "i", "add":
		if len(args) == 0 {
			return "npm install", nil
		}
		name, ver := splitNPMArg(args[0])
		if err := validNPMPkg(name); err != nil {
			return "", err
		}
		spec := name
		if ver != "" {
			if err := validConstraint(ver); err != nil {
				return "", err
			}
			spec += "@" + ver
		}
		cmd := "npm install --no-fund --no-audit " + shellQuote(spec)
		if dev {
			cmd += " --save-dev"
		}
		return cmd, nil
	case "uninstall", "remove", "rm":
		if len(args) == 0 {
			return "", fmt.Errorf("package name required")
		}
		name, _ := splitNPMArg(args[0])
		if err := validNPMPkg(name); err != nil {
			return "", err
		}
		return "npm uninstall --no-fund --no-audit " + shellQuote(name), nil
	case "update", "upgrade":
		if len(args) == 0 {
			return "npm update", nil
		}
		name, _ := splitNPMArg(args[0])
		if err := validNPMPkg(name); err != nil {
			return "", err
		}
		return "npm update --no-fund --no-audit " + shellQuote(name), nil
	case "run", "run-script":
		if len(args) == 0 {
			return "", fmt.Errorf("script name required")
		}
		if !validNPMScript(args[0], scripts) {
			return "", fmt.Errorf("unknown npm script %q", args[0])
		}
		return "npm run --silent " + shellQuote(args[0]), nil
	case "build", "dev", "start", "test", "lint":
		if !validNPMScript(fields[0], scripts) && fields[0] != "build" {
			return "", fmt.Errorf("unknown npm script %q", fields[0])
		}
		if fields[0] == "build" {
			return "npm run build", nil
		}
		return "npm run --silent " + shellQuote(fields[0]), nil
	default:
		if validNPMPkg(fields[0]) == nil {
			name, ver := splitNPMArg(fields[0])
			spec := name
			if ver != "" {
				if err := validConstraint(ver); err != nil {
					return "", err
				}
				spec += "@" + ver
			}
			return "npm install --no-fund --no-audit " + shellQuote(spec), nil
		}
		return "", fmt.Errorf("allowed: install, uninstall, update, run")
	}
}

func validNPMScript(name string, scripts []string) bool {
	if !regexp.MustCompile(`^[A-Za-z0-9._:-]+$`).MatchString(name) {
		return false
	}
	if len(scripts) == 0 {
		return true
	}
	for _, s := range scripts {
		if s == name {
			return true
		}
	}
	return false
}

func validComposerPkg(name string) error {
	if !composerPkgRe.MatchString(name) || strings.Contains(name, "..") {
		return fmt.Errorf("invalid composer package")
	}
	return nil
}

func validNPMPkg(name string) error {
	n := strings.ToLower(name)
	if !npmPkgRe.MatchString(n) || strings.Contains(n, "..") {
		return fmt.Errorf("invalid npm package")
	}
	return nil
}

func validConstraint(v string) error {
	if v == "" || len(v) > 40 || strings.ContainsAny(v, " \t\n;&|`$()<>") {
		return fmt.Errorf("invalid version constraint")
	}
	return nil
}

func splitComposerArg(s string) (pkg, ver string) {
	slash := strings.Index(s, "/")
	if slash < 0 {
		return s, ""
	}
	rest := s[slash+1:]
	if i := strings.Index(rest, ":"); i >= 0 {
		return s[:slash+1+i], rest[i+1:]
	}
	return s, ""
}

func splitNPMArg(s string) (name, ver string) {
	if strings.HasPrefix(s, "@") {
		if i := strings.LastIndex(s, "@"); i > 0 {
			return s[:i], s[i+1:]
		}
		return s, ""
	}
	if i := strings.Index(s, "@"); i > 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

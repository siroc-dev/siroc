package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/secret"
	"github.com/siroc-dev/siroc/internal/validate"
)

type directAdminPlan struct {
	Manifest         *rpc.BackupManifest
	Copies           []directAdminCopy
	Dumps            []directAdminDump
	ResetDBPasswords int
}

type directAdminCopy struct {
	Src string
	Rel string
}

type directAdminDump struct {
	Src  string
	Name string
}

func ParseDirectAdmin(root string) (*directAdminPlan, error) {
	base, userConf, err := locateDirectAdmin(root)
	if err != nil {
		return nil, err
	}
	username := strings.ToLower(strings.TrimSpace(userConf["username"]))
	if err := validate.LinuxUser(username); err != nil {
		return nil, err
	}
	primary, _ := validate.ASCIIHost(userConf["domain"])
	domains := directAdminDomains(base, primary)
	sites := make([]rpc.BackupSite, 0, len(domains))
	siteNames := map[string]bool{}
	var copies []directAdminCopy
	for _, domain := range domains {
		doc := "/home/" + username + "/domains/" + domain + "/public_html"
		if st, err := os.Stat(filepath.Join(base, "domains", domain, "public")); err == nil && st.IsDir() {
			if _, err := os.Stat(filepath.Join(base, "domains", domain, "public_html")); err != nil {
				doc = "/home/" + username + "/domains/" + domain + "/public"
			}
		}
		aliases := directAdminAliases(base, domain, siteNames)
		sites = append(sites, rpc.BackupSite{
			Domain:     domain,
			DocRoot:    doc,
			PHPVersion: "8.3",
			Enabled:    true,
			Aliases:    aliases,
			Kind:       "php",
			WAF:        true,
		})
		siteNames[domain] = true
		src := filepath.Join(base, "domains", domain)
		if st, err := os.Stat(src); err == nil && st.IsDir() {
			copies = append(copies, directAdminCopy{Src: src, Rel: "domains/" + domain})
		}
	}
	sites = append(sites, directAdminSubdomains(base, username, sites)...)
	for _, st := range sites {
		siteNames[st.Domain] = true
	}
	for i := range sites {
		sites[i].Aliases = withoutDomains(sites[i].Aliases, siteNames, sites[i].Domain)
	}
	sort.SliceStable(sites, func(i, j int) bool {
		if sites[i].Domain == primary {
			return true
		}
		if sites[j].Domain == primary {
			return false
		}
		return sites[i].Domain < sites[j].Domain
	})
	dbs, dumps, resets := directAdminDatabases(base)
	ftps, ftpOn := directAdminFTP(base, username)
	return &directAdminPlan{
		Manifest: &rpc.BackupManifest{
			Version:  1,
			Username: username,
			Account: rpc.BackupAccount{
				Username:     username,
				KeepPassword: true,
				SkipFlags:    true,
				FTP:          ftpOn,
				WAFEnabled:   true,
			},
			Sites:     sites,
			Databases: dbs,
			FTP:       ftps,
		},
		Copies:           copies,
		Dumps:            dumps,
		ResetDBPasswords: resets,
	}, nil
}

func locateDirectAdmin(root string) (string, map[string]string, error) {
	var candidates []string
	candidates = append(candidates, root)
	ents, err := os.ReadDir(root)
	if err == nil {
		for _, e := range ents {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && !skipDirectAdminDir(e.Name()) {
				candidates = append(candidates, filepath.Join(root, e.Name()))
			}
		}
	}
	for _, base := range candidates {
		for _, rel := range []string{"user.conf", filepath.Join("backup", "user.conf")} {
			kv, err := parseConfFile(filepath.Join(base, rel))
			if err != nil {
				continue
			}
			if strings.TrimSpace(kv["username"]) != "" {
				return base, kv, nil
			}
		}
	}
	return "", nil, errNotDirectAdmin
}

var errNotDirectAdmin = errString("not a DirectAdmin user backup")

type errString string

func (e errString) Error() string { return string(e) }

func directAdminDomains(base, primary string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(name string, force bool) {
		ascii, err := validate.ASCIIHost(name)
		if err != nil || seen[ascii] {
			return
		}
		if !force && !directAdminHasWeb(base, ascii) {
			return
		}
		seen[ascii] = true
		out = append(out, ascii)
	}
	if primary != "" {
		add(primary, true)
	}
	list, _ := os.ReadFile(filepath.Join(base, "domains.list"))
	for _, line := range strings.Split(string(list), "\n") {
		add(line, false)
	}
	if ents, err := os.ReadDir(filepath.Join(base, "domains")); err == nil {
		for _, e := range ents {
			name := e.Name()
			if e.IsDir() {
				add(name, false)
				continue
			}
			if strings.HasSuffix(strings.ToLower(name), ".conf") && !strings.EqualFold(name, "domain.conf") {
				add(strings.TrimSuffix(name, filepath.Ext(name)), false)
			}
		}
	}
	if ents, err := os.ReadDir(filepath.Join(base, "backup")); err == nil {
		for _, e := range ents {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(strings.ToLower(name), ".conf") || name == "user.conf" {
				continue
			}
			add(strings.TrimSuffix(name, filepath.Ext(name)), false)
		}
	}
	sort.Strings(out)
	return out
}

func directAdminHasWeb(base, domain string) bool {
	for _, rel := range []string{
		filepath.Join("domains", domain),
		filepath.Join("domains", domain+".conf"),
		filepath.Join("backup", domain+".conf"),
	} {
		if _, err := os.Stat(filepath.Join(base, rel)); err == nil {
			return true
		}
	}
	return false
}

func directAdminAliases(base, domain string, taken map[string]bool) []string {
	var files []string
	for _, rel := range []string{
		filepath.Join("backup", domain+".pointers"),
		filepath.Join("domains", domain, "domain.pointers"),
		filepath.Join("domains", domain, "pointers"),
		filepath.Join("domains", domain+".pointers"),
	} {
		files = append(files, filepath.Join(base, rel))
	}
	seen := map[string]bool{}
	var out []string
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(strings.TrimRight(line, "\r"))
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			field := strings.Fields(line)
			if len(field) == 0 {
				continue
			}
			name := field[0]
			if i := strings.IndexByte(name, '='); i >= 0 {
				name = name[:i]
			}
			ascii, err := validate.ASCIIAlias(name)
			if err != nil || ascii == domain || taken[ascii] || seen[ascii] {
				continue
			}
			seen[ascii] = true
			out = append(out, ascii)
			if len(out) >= validate.MaxDomainAliases {
				return out
			}
		}
	}
	sort.Strings(out)
	return out
}

func directAdminSubdomains(base, username string, sites []rpc.BackupSite) []rpc.BackupSite {
	have := map[string]bool{}
	for _, st := range sites {
		have[st.Domain] = true
	}
	var extra []rpc.BackupSite
	for _, st := range sites {
		var files []string
		for _, rel := range []string{
			filepath.Join("backup", st.Domain+".subdomains"),
			filepath.Join("domains", st.Domain, "domain.subdomains"),
			filepath.Join("domains", st.Domain, "subdomains"),
			filepath.Join("domains", st.Domain+".subdomains"),
		} {
			files = append(files, filepath.Join(base, rel))
		}
		for _, path := range files {
			b, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			for _, line := range strings.Split(string(b), "\n") {
				label := strings.TrimSpace(strings.TrimRight(line, "\r"))
				if label == "" || strings.HasPrefix(label, "#") {
					continue
				}
				label = strings.Fields(label)[0]
				if i := strings.IndexByte(label, '='); i >= 0 {
					label = label[:i]
				}
				label = strings.ToLower(label)
				if label == "" || strings.Contains(label, "/") {
					continue
				}
				host := label
				folder := label
				if strings.Contains(label, ".") {
					host = label
					folder = strings.TrimSuffix(label, "."+st.Domain)
					if folder == label || strings.Contains(folder, ".") {
						continue
					}
				} else {
					host = label + "." + st.Domain
				}
				ascii, err := validate.ASCIIHost(host)
				if err != nil || have[ascii] {
					continue
				}
				dir := filepath.Join(base, "domains", st.Domain, "public_html", folder)
				if stInfo, err := os.Stat(dir); err != nil || !stInfo.IsDir() {
					continue
				}
				have[ascii] = true
				extra = append(extra, rpc.BackupSite{
					Domain:     ascii,
					DocRoot:    "/home/" + username + "/domains/" + st.Domain + "/public_html/" + folder,
					PHPVersion: "8.3",
					Enabled:    true,
					Kind:       "php",
					WAF:        true,
				})
			}
		}
	}
	return extra
}

func withoutDomains(aliases []string, taken map[string]bool, self string) []string {
	var out []string
	for _, name := range aliases {
		if name == self || taken[name] {
			continue
		}
		out = append(out, name)
	}
	return out
}

func directAdminDatabases(base string) ([]rpc.BackupDatabase, []directAdminDump, int) {
	var dirs []string
	for _, rel := range []string{"mysql", filepath.Join("backup", "mysql")} {
		if st, err := os.Stat(filepath.Join(base, rel)); err == nil && st.IsDir() {
			dirs = append(dirs, filepath.Join(base, rel))
		}
	}
	seen := map[string]bool{}
	var dbs []rpc.BackupDatabase
	var dumps []directAdminDump
	resets := 0
	for _, dir := range dirs {
		ents, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".sql") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			if validate.DBIdent(name) != nil || seen[name] {
				continue
			}
			conf, _ := parseConfFile(filepath.Join(dir, name+".conf"))
			user := firstConf(conf, "user", "dbuser", "userdb")
			if user == "" || validate.DBIdent(user) != nil {
				user = name
			}
			pass := firstConf(conf, "passwd", "password", "pass")
			if !usableDBPassword(pass) {
				generated, err := secret.RandomPassword(16)
				if err != nil {
					continue
				}
				pass = generated
				resets++
			}
			seen[name] = true
			dbs = append(dbs, rpc.BackupDatabase{DBName: name, DBUser: user, Password: pass, Engine: "mysql"})
			dumps = append(dumps, directAdminDump{Src: filepath.Join(dir, e.Name()), Name: name})
		}
	}
	sort.Slice(dbs, func(i, j int) bool { return dbs[i].DBName < dbs[j].DBName })
	sort.Slice(dumps, func(i, j int) bool { return dumps[i].Name < dumps[j].Name })
	return dbs, dumps, resets
}

func firstConf(conf map[string]string, keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(conf[key]); v != "" {
			return v
		}
	}
	return ""
}

func usableDBPassword(pw string) bool {
	if len(pw) < 1 || len(pw) > 72 {
		return false
	}
	low := strings.ToLower(pw)
	if strings.HasPrefix(pw, "$") || strings.Contains(low, "encrypt") {
		return false
	}
	for _, r := range pw {
		if r < 32 || r > 126 {
			return false
		}
	}
	return true
}

func parseConfFile(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if key == "" {
			continue
		}
		out[key] = val
	}
	return out, nil
}

func NestedUserArchives(root string) []string {
	dirs := []string{root}
	ents, err := os.ReadDir(root)
	if err == nil {
		for _, e := range ents {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && !skipDirectAdminDir(e.Name()) {
				dirs = append(dirs, filepath.Join(root, e.Name()))
			}
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, dir := range dirs {
		ents, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if e.IsDir() || !isUserArchiveName(e.Name()) {
				continue
			}
			path := filepath.Join(dir, e.Name())
			if seen[path] {
				continue
			}
			seen[path] = true
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

func skipDirectAdminDir(name string) bool {
	switch strings.ToLower(name) {
	case "domains", "backup", "email", "imap", "mysql", "siroc-meta", "public_html", "private_html":
		return true
	default:
		return false
	}
}

func isUserArchiveName(name string) bool {
	n := strings.ToLower(name)
	return strings.HasSuffix(n, ".tar.zst") || strings.HasSuffix(n, ".tzst") || strings.HasSuffix(n, ".tar.gz") || strings.HasSuffix(n, ".tgz") || strings.HasSuffix(n, ".tar")
}

func (p *directAdminPlan) selectDomains(domains []string, php string) error {
	if p == nil || p.Manifest == nil {
		return fmt.Errorf("backup has none of the selected domains")
	}
	want := map[string]bool{}
	for _, d := range domains {
		ascii, err := validate.ASCIIHost(d)
		if err == nil {
			want[ascii] = true
		}
	}
	if len(want) == 0 {
		return fmt.Errorf("no domains selected")
	}
	var sites []rpc.BackupSite
	for _, st := range p.Manifest.Sites {
		if !want[st.Domain] && !docrootUnder(st.DocRoot, want) {
			continue
		}
		if php != "" {
			st.PHPVersion = php
		}
		st.Enabled = true
		st.SSL = true
		st.SSLKind = "local"
		sites = append(sites, st)
	}
	if len(sites) == 0 {
		return fmt.Errorf("backup has none of the selected domains")
	}
	var copies []directAdminCopy
	for _, c := range p.Copies {
		domain := strings.TrimPrefix(strings.ReplaceAll(c.Rel, "\\", "/"), "domains/")
		if i := strings.IndexByte(domain, '/'); i >= 0 {
			domain = domain[:i]
		}
		for _, st := range sites {
			if strings.Contains(strings.ReplaceAll(st.DocRoot, "\\", "/"), "/domains/"+domain+"/") {
				copies = append(copies, c)
				break
			}
		}
	}
	p.Manifest.Sites = sites
	p.Copies = copies
	return nil
}

func docrootUnder(docroot string, want map[string]bool) bool {
	docroot = strings.ReplaceAll(docroot, "\\", "/")
	i := strings.Index(docroot, "/domains/")
	if i < 0 {
		return false
	}
	rest := docroot[i+len("/domains/"):]
	parent := rest
	if j := strings.IndexByte(rest, '/'); j >= 0 {
		parent = rest[:j]
	}
	return parent != "" && want[parent]
}

func directAdminFTP(base, owner string) ([]rpc.BackupFTP, bool) {
	if strings.TrimSpace(owner) == "" {
		return nil, false
	}
	var b []byte
	var err error
	for _, rel := range []string{"ftp.passwd", filepath.Join("backup", "ftp.passwd")} {
		b, err = os.ReadFile(filepath.Join(base, rel))
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, false
	}
	var out []rpc.BackupFTP
	seen := map[string]bool{}
	main := false
	for _, line := range strings.Split(string(b), "\n") {
		login, home := parseFTPLine(line)
		if login == "" {
			continue
		}
		if strings.EqualFold(login, owner) {
			main = true
			continue
		}
		full, ok := panelFTPLogin(owner, login)
		if !ok || seen[full] {
			continue
		}
		pw, err := secret.RandomPassword(16)
		if err != nil {
			continue
		}
		seen[full] = true
		out = append(out, rpc.BackupFTP{Login: full, Home: home, Password: pw})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Login < out[j].Login })
	return out, main || len(out) > 0
}

func parseFTPLine(line string) (login, home string) {
	line = strings.TrimSpace(strings.TrimRight(line, "\r"))
	if line == "" || strings.HasPrefix(line, "#") {
		return "", ""
	}
	parts := strings.Split(line, ":")
	if len(parts) < 2 {
		return "", ""
	}
	login = strings.ToLower(strings.TrimSpace(parts[0]))
	if len(parts) >= 7 && strings.HasPrefix(strings.TrimSpace(parts[5]), "/") {
		return login, strings.TrimSpace(parts[5])
	}
	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "/") && !ftpShell(part) {
			home = part
		}
	}
	return login, home
}

func ftpShell(path string) bool {
	switch path {
	case "/bin/false", "/bin/nologin", "/sbin/nologin", "/usr/sbin/nologin", "/bin/bash", "/bin/sh":
		return true
	default:
		return false
	}
}

func panelFTPLogin(owner, login string) (string, bool) {
	owner = strings.ToLower(strings.TrimSpace(owner))
	login = strings.ToLower(strings.TrimSpace(login))
	if owner == "" || login == "" || login == owner {
		return "", false
	}
	suffix := login
	if strings.HasPrefix(login, owner+"_") {
		suffix = strings.TrimPrefix(login, owner+"_")
	} else if strings.HasPrefix(login, owner) {
		suffix = strings.TrimPrefix(strings.TrimPrefix(login, owner), "_")
	}
	full, err := validate.FTPLogin(owner, suffix)
	if err != nil || full != owner+"_"+suffix {
		return "", false
	}
	return full, true
}

func directAdminMessage(username string, sites, dbs, ftps, resets int, created bool) string {
	msg := "restored " + username + " from DirectAdmin"
	msg += " (" + itoa(sites) + " sites, " + itoa(dbs) + " databases, " + itoa(ftps) + " ftp users)"
	if created {
		msg = "created user " + username + " and " + msg
	}
	msg += ". Mail, DNS, and certificates were not imported."
	if resets > 0 {
		msg += " Reset " + itoa(resets) + " database passwords because the backup had none usable."
	}
	if ftps > 0 {
		msg += " FTP login passwords were reset because DirectAdmin only stores a hash. Open FTP to see the new passwords."
	}
	msg += " Set the account password under Accounts if this user is new to the panel."
	return msg
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

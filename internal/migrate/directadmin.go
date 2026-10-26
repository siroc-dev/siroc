package migrate

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/validate"
)

const maxArchiveBytes = 20 << 30

type fileEntry struct {
	name string
	size string
}

func ParseEndpoint(raw string, port int) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("host is required")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "", fmt.Errorf("invalid host")
	}
	if u.Port() != "" {
		n, err := strconv.Atoi(u.Port())
		if err != nil || n < 1 || n > 65535 {
			return "", fmt.Errorf("invalid port")
		}
		port = n
	}
	if port == 0 {
		port = 2222
	}
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("invalid port")
	}
	return "https://" + net.JoinHostPort(u.Hostname(), strconv.Itoa(port)), nil
}

func DAError(v url.Values) string {
	if v.Get("error") != "1" && !strings.EqualFold(v.Get("error"), "yes") {
		return ""
	}
	msg := strings.TrimSpace(v.Get("text"))
	if msg == "" {
		msg = strings.TrimSpace(v.Get("details"))
	}
	if msg == "" {
		msg = "DirectAdmin refused the request"
	}
	return msg
}

func UserNames(v url.Values) []string {
	skip := map[string]bool{"error": true, "text": true, "details": true}
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || strings.Contains(s, ".") || strings.ContainsAny(s, "/\\ ") || seen[s] || skip[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	for _, key := range []string{"list[]", "list"} {
		for _, s := range v[key] {
			add(s)
		}
	}
	sort.Strings(out)
	return out
}

func DomainNames(v url.Values) []string {
	skip := map[string]bool{"error": true, "text": true, "details": true, "list": true, "list[]": true}
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		s = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(s, ".")))
		if s == "" || !strings.Contains(s, ".") || seen[s] {
			return
		}
		ascii, err := validate.ASCIIHost(s)
		if err != nil {
			return
		}
		if seen[ascii] {
			return
		}
		seen[ascii] = true
		out = append(out, ascii)
	}
	for _, s := range v["list[]"] {
		add(s)
	}
	for _, s := range v["list"] {
		add(s)
	}
	for k, vals := range v {
		if skip[k] {
			continue
		}
		add(k)
		for _, s := range vals {
			add(s)
		}
	}
	sort.Strings(out)
	return out
}

func isArchive(name string) bool {
	n := strings.ToLower(name)
	return strings.HasSuffix(n, ".tar.gz") || strings.HasSuffix(n, ".tgz") || strings.HasSuffix(n, ".tar.zst") || strings.HasSuffix(n, ".tar")
}

func ArchiveNames(v url.Values) []fileEntry {
	return FileEntries(v)
}

func FileEntries(v url.Values) []fileEntry {
	var names []string
	for _, key := range []string{"list[]", "list"} {
		names = append(names, v[key]...)
	}
	sizes := v["size[]"]
	if len(sizes) == 0 {
		sizes = v["size"]
	}
	seen := map[string]bool{}
	var out []fileEntry
	add := func(s, size string) {
		s = strings.TrimSpace(s)
		base := s
		if i := strings.LastIndexAny(s, "/\\"); i >= 0 {
			base = s[i+1:]
		}
		if !isArchive(base) || seen[base] || strings.Contains(base, "..") {
			return
		}
		seen[base] = true
		out = append(out, fileEntry{name: base, size: strings.TrimSpace(size)})
	}
	for i, s := range names {
		size := ""
		if i < len(sizes) {
			size = sizes[i]
		}
		add(s, size)
	}
	for k := range v {
		add(k, "")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// NewStableArchive returns a backup that was not in the earlier snapshot once
// its name, and size when DirectAdmin sends one, stays the same for two looks.
func NewStableArchive(before, prev map[string]string, now []fileEntry) (string, map[string]string) {
	next := map[string]string{}
	for _, e := range now {
		if !isArchive(e.name) {
			continue
		}
		next[e.name] = e.size
		old, known := before[e.name]
		if known && (e.size == "" || e.size == old || e.size == "0") {
			continue
		}
		if e.size == "0" {
			continue
		}
		if p, seen := prev[e.name]; seen && p == e.size {
			return e.name, next
		}
	}
	return "", next
}

type Client struct {
	Base     string
	User     string
	Pass     string
	Insecure bool
	http     *http.Client
	long     *http.Client
}

func Connect(conn rpc.DAConn) (*Client, error) {
	user := strings.TrimSpace(conn.Username)
	if user == "" || strings.Contains(user, ":") || strings.Contains(user, "|") {
		return nil, fmt.Errorf("username is required")
	}
	if strings.TrimSpace(conn.Password) == "" {
		return nil, fmt.Errorf("password or login key is required")
	}
	base, err := ParseEndpoint(conn.Host, conn.Port)
	if err != nil {
		return nil, err
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: conn.Insecure}
	return &Client{
		Base:     base,
		User:     user,
		Pass:     conn.Password,
		Insecure: conn.Insecure,
		http: &http.Client{
			Timeout:   30 * time.Second,
			Transport: tr,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		long: &http.Client{
			Timeout:   45 * time.Minute,
			Transport: tr,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func Discover(conn rpc.DAConn) ([]rpc.DASite, error) {
	if sshMode(conn) {
		return DiscoverSSH(conn)
	}
	c, err := Connect(conn)
	if err != nil {
		return nil, err
	}
	return c.Discover()
}

func (c *Client) Discover() ([]rpc.DASite, error) {
	users, uerr := c.listUsers()
	if uerr != nil || len(users) == 0 {
		domains, err := c.listDomains("")
		if err != nil {
			if uerr != nil {
				return nil, uerr
			}
			return nil, err
		}
		var sites []rpc.DASite
		for _, d := range domains {
			sites = append(sites, rpc.DASite{Domain: d, Owner: c.User})
		}
		if len(sites) == 0 {
			return nil, fmt.Errorf("DirectAdmin returned no websites")
		}
		return sites, nil
	}
	var sites []rpc.DASite
	for _, user := range users {
		domains, err := c.listDomains(user)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", user, err)
		}
		for _, d := range domains {
			sites = append(sites, rpc.DASite{Domain: d, Owner: user})
		}
	}
	if len(sites) == 0 {
		return nil, fmt.Errorf("DirectAdmin returned no websites")
	}
	return sites, nil
}

func (c *Client) listUsers() ([]string, error) {
	v, err := c.get("/CMD_API_SHOW_USERS")
	if err != nil {
		return nil, err
	}
	if msg := DAError(v); msg != "" {
		return nil, fmt.Errorf("%s", msg)
	}
	return UserNames(v), nil
}

func (c *Client) listDomains(user string) ([]string, error) {
	path := "/CMD_API_SHOW_DOMAINS"
	if user != "" && user != c.User {
		path = "/CMD_API_SHOW_USER_DOMAINS?user=" + url.QueryEscape(user)
	}
	v, err := c.as(user).get(path)
	if err != nil {
		return nil, err
	}
	if msg := DAError(v); msg != "" {
		return nil, fmt.Errorf("%s", msg)
	}
	return DomainNames(v), nil
}

func (c *Client) as(owner string) *Client {
	owner = strings.TrimSpace(owner)
	if owner == "" || owner == c.User || strings.Contains(c.User, "|") {
		return c
	}
	cp := *c
	cp.User = c.User + "|" + owner
	return &cp
}

func (c *Client) Pull(owner, domain, destDir string, progress func(step string, pct int)) (string, error) {
	owner = strings.TrimSpace(owner)
	if owner == "" || strings.Contains(owner, "/") || strings.Contains(owner, "..") {
		return "", fmt.Errorf("DirectAdmin user %q cannot be used", owner)
	}
	who := c.as(owner)
	dirs := []string{"/home/" + owner + "/backups", "/home/" + owner + "/user_backups"}
	before := map[string]map[string]string{}
	for _, dir := range dirs {
		entries, err := who.files(dir)
		if err != nil {
			continue
		}
		before[dir] = map[string]string{}
		for _, e := range entries {
			before[dir][e.name] = e.size
		}
	}
	if progress != nil {
		progress("backup", 15)
	}
	if err := who.startBackup(domain); err != nil {
		return "", err
	}
	if progress != nil {
		progress("backup", 25)
	}
	dir, name, err := who.waitArchive(dirs, before, progress)
	if err != nil {
		return "", err
	}
	if progress != nil {
		progress("download", 40)
	}
	safe := strings.Map(func(ch rune) rune {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' {
			return ch
		}
		return '_'
	}, strings.ToLower(domain))
	dest := filepath.Join(destDir, "da-"+safe+"-"+time.Now().UTC().Format("20060102-150405")+archiveSuffix(name))
	if err := who.download(dir, name, dest, progress); err != nil {
		_ = os.Remove(dest)
		return "", err
	}
	return dest, nil
}

func archiveSuffix(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.HasSuffix(n, ".tar.gz"):
		return ".tar.gz"
	case strings.HasSuffix(n, ".tar.zst"):
		return ".tar.zst"
	case strings.HasSuffix(n, ".tgz"):
		return ".tgz"
	default:
		return ".tar"
	}
}

func (c *Client) startBackup(domain string) error {
	form := url.Values{}
	form.Set("select0", domain)
	form.Set("option0", "domain")
	form.Set("option1", "subdomain")
	form.Set("option2", "database")
	form.Set("option3", "ftp")
	form.Set("where", "local")
	var last error
	for _, action := range []string{"backup", "create"} {
		form.Set("action", action)
		v, err := c.post("/CMD_API_SITE_BACKUP", form)
		if err != nil {
			last = err
			continue
		}
		if msg := DAError(v); msg != "" {
			last = fmt.Errorf("%s", msg)
			continue
		}
		return nil
	}
	if last == nil {
		last = fmt.Errorf("DirectAdmin did not start the backup")
	}
	return last
}

func (c *Client) waitArchive(dirs []string, before map[string]map[string]string, progress func(step string, pct int)) (string, string, error) {
	prev := map[string]map[string]string{}
	deadline := time.Now().Add(30 * time.Minute)
	for {
		for _, dir := range dirs {
			entries, err := c.files(dir)
			if err != nil {
				continue
			}
			if _, ok := before[dir]; !ok {
				before[dir] = map[string]string{}
				for _, e := range entries {
					before[dir][e.name] = e.size
				}
				continue
			}
			name, next := NewStableArchive(before[dir], prev[dir], entries)
			prev[dir] = next
			if name != "" {
				return dir, name, nil
			}
		}
		if time.Now().After(deadline) {
			return "", "", fmt.Errorf("DirectAdmin did not finish the backup")
		}
		if progress != nil {
			progress("backup", 30)
		}
		time.Sleep(5 * time.Second)
	}
}

func (c *Client) files(dir string) ([]fileEntry, error) {
	v, err := c.get("/CMD_API_FILE_MANAGER?path=" + url.QueryEscape(dir))
	if err != nil {
		return nil, err
	}
	if msg := DAError(v); msg != "" {
		return nil, fmt.Errorf("%s", msg)
	}
	return FileEntries(v), nil
}

func (c *Client) download(dir, name, dest string, progress func(step string, pct int)) error {
	if !isArchive(name) || strings.Contains(name, "..") || strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("unexpected backup name")
	}
	path := dir + "/" + name
	req, err := http.NewRequest(http.MethodGet, c.Base+"/CMD_FILE_MANAGER"+path, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.User, c.Pass)
	res, err := c.long.Do(req)
	if err != nil {
		return fmt.Errorf("download failed")
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return fmt.Errorf("DirectAdmin rejected the login")
	}
	if res.StatusCode >= 400 {
		return fmt.Errorf("DirectAdmin did not return the backup file")
	}
	if res.ContentLength > maxArchiveBytes {
		return fmt.Errorf("backup is larger than 20GB")
	}
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	defer f.Close()
	buf := make([]byte, 32*1024)
	var written int64
	checked := false
	for {
		n, rerr := res.Body.Read(buf)
		if n > 0 {
			if !checked {
				checked = true
				head := strings.ToLower(string(buf[:n]))
				if strings.HasPrefix(strings.TrimSpace(head), "<") || strings.Contains(head, "<html") || strings.HasPrefix(strings.TrimSpace(head), "error=") {
					return fmt.Errorf("DirectAdmin did not return the backup file")
				}
			}
			if written+int64(n) > maxArchiveBytes {
				return fmt.Errorf("backup is larger than 20GB")
			}
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			written += int64(n)
			if progress != nil && res.ContentLength > 0 {
				pct := 40 + int(written*30/res.ContentLength)
				if pct > 70 {
					pct = 70
				}
				progress("download", pct)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return fmt.Errorf("download failed")
		}
	}
	if written == 0 {
		return fmt.Errorf("DirectAdmin did not return the backup file")
	}
	return nil
}

func (c *Client) get(path string) (url.Values, error) {
	req, err := http.NewRequest(http.MethodGet, c.Base+path, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.User, c.Pass)
	return c.read(req, c.http)
}

func (c *Client) post(path string, form url.Values) (url.Values, error) {
	req, err := http.NewRequest(http.MethodPost, c.Base+path, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(c.User, c.Pass)
	return c.read(req, c.http)
}

func (c *Client) read(req *http.Request, client *http.Client) (url.Values, error) {
	if client == nil {
		client = c.http
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach DirectAdmin")
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden || res.StatusCode == http.StatusFound || res.StatusCode == http.StatusSeeOther {
		return nil, fmt.Errorf("DirectAdmin rejected the login")
	}
	text := strings.TrimSpace(string(body))
	if strings.HasPrefix(strings.ToLower(text), "<") || strings.Contains(strings.ToLower(text), "<html") {
		return nil, fmt.Errorf("DirectAdmin rejected the login")
	}
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("DirectAdmin returned %d", res.StatusCode)
	}
	text = strings.ReplaceAll(text, "\r\n", "&")
	text = strings.ReplaceAll(text, "\n", "&")
	v, err := url.ParseQuery(text)
	if err != nil {
		return nil, fmt.Errorf("DirectAdmin returned an unreadable response")
	}
	return v, nil
}

package migrate

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/validate"
)

const (
	sshIdle    = 15 * time.Minute
	sshSizeMax = 2 * time.Hour
	sshPackMax = 8 * time.Hour
	sshPullMax = 12 * time.Hour
)

func sshMode(conn rpc.DAConn) bool {
	return strings.EqualFold(strings.TrimSpace(conn.Mode), "ssh")
}

func ParseSSHHost(raw string, port int) (string, int, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "ssh://")
	raw = strings.TrimPrefix(raw, "https://")
	raw = strings.TrimPrefix(raw, "http://")
	if i := strings.Index(raw, "/"); i >= 0 {
		raw = raw[:i]
	}
	if at := strings.LastIndex(raw, "@"); at >= 0 {
		raw = raw[at+1:]
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", 0, fmt.Errorf("host is required")
	}
	host := raw
	if h, p, err := net.SplitHostPort(raw); err == nil {
		host = h
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return "", 0, fmt.Errorf("invalid port")
		}
		port = n
	}
	host = strings.Trim(host, "[]")
	if host == "" || strings.ContainsAny(host, " \t/") {
		return "", 0, fmt.Errorf("invalid host")
	}
	if port == 0 {
		port = 22
	}
	if port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("invalid port")
	}
	return host, port, nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func ParseSSHSites(text string) ([]rpc.DASite, error) {
	seen := map[string]bool{}
	var sites []rpc.DASite
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		owner, domain, ok := strings.Cut(line, "\t")
		if !ok {
			owner, domain, ok = strings.Cut(line, " ")
		}
		if !ok {
			continue
		}
		owner = strings.ToLower(strings.TrimSpace(owner))
		if owner == "" || strings.ContainsAny(owner, " \t/\\.") {
			continue
		}
		ascii, err := validate.ASCIIHost(domain)
		if err != nil {
			continue
		}
		key := owner + "\n" + ascii
		if seen[key] {
			continue
		}
		seen[key] = true
		sites = append(sites, rpc.DASite{Domain: ascii, Owner: owner})
	}
	sort.Slice(sites, func(i, j int) bool {
		if sites[i].Domain == sites[j].Domain {
			return sites[i].Owner < sites[j].Owner
		}
		return sites[i].Domain < sites[j].Domain
	})
	if len(sites) == 0 {
		return nil, fmt.Errorf("DirectAdmin returned no websites")
	}
	return sites, nil
}

func sshUser(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, " \t:/\\") {
		return fmt.Errorf("username is required")
	}
	return nil
}

type sshConn struct {
	cli         *ssh.Client
	pass        string
	useSudo     bool
	useSudoPass bool
}

func DialSSH(conn rpc.DAConn) (*sshConn, error) {
	if err := sshUser(conn.Username); err != nil {
		return nil, err
	}
	if strings.TrimSpace(conn.Password) == "" && strings.TrimSpace(conn.PrivateKey) == "" {
		return nil, fmt.Errorf("enter a password or upload a PPK file")
	}
	if len(conn.PrivateKey) > maxPrivateKeyBytes {
		return nil, fmt.Errorf("private key file is too large")
	}
	if strings.ContainsAny(conn.Password, "\r\n") {
		return nil, fmt.Errorf("password cannot contain a new line")
	}
	host, port, err := ParseSSHHost(conn.Host, conn.Port)
	if err != nil {
		return nil, err
	}
	var methods []ssh.AuthMethod
	if strings.TrimSpace(conn.PrivateKey) != "" {
		signer, err := ParsePrivateKey(conn.PrivateKey, conn.KeyPassphrase)
		if err != nil {
			return nil, err
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if pw := conn.Password; pw != "" {
		methods = append(methods, ssh.Password(pw), ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
			answers := make([]string, len(questions))
			for i := range answers {
				answers[i] = pw
			}
			return answers, nil
		}))
	}
	cfg := &ssh.ClientConfig{
		User:            strings.TrimSpace(conn.Username),
		Auth:            methods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         20 * time.Second,
	}
	cli, err := ssh.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(port)), cfg)
	if err != nil {
		return nil, sshDialError(err)
	}
	go keepSSHAlive(cli)
	return &sshConn{cli: cli, pass: conn.Password}, nil
}

func keepSSHAlive(cli *ssh.Client) {
	tick := time.NewTicker(20 * time.Second)
	defer tick.Stop()
	for range tick.C {
		_, _, err := cli.SendRequest("keepalive@openssh.com", true, nil)
		if err != nil {
			return
		}
	}
}

func sshDialError(err error) error {
	low := strings.ToLower(err.Error())
	switch {
	case strings.Contains(low, "unable to authenticate"), strings.Contains(low, "permission denied"):
		return fmt.Errorf("SSH login failed")
	case strings.Contains(low, "refused"), strings.Contains(low, "timeout"), strings.Contains(low, "no such host"), strings.Contains(low, "network is unreachable"):
		return fmt.Errorf("cannot reach SSH on that host and port")
	default:
		return fmt.Errorf("SSH login failed")
	}
}

func (c *sshConn) Close() error {
	if c == nil || c.cli == nil {
		return nil
	}
	return c.cli.Close()
}

func CheckSSH(conn rpc.DAConn) error {
	c, err := DialSSH(conn)
	if err != nil {
		return err
	}
	defer c.Close()
	return c.prepare()
}

func DiscoverSSH(conn rpc.DAConn) ([]rpc.DASite, error) {
	c, err := DialSSH(conn)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	if err := c.prepare(); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := c.bash(2*time.Minute, listSitesScript, nil, &out); err != nil {
		return nil, err
	}
	return ParseSSHSites(out.String())
}

func Measure(req rpc.DASizeReq) (rpc.DASizeList, error) {
	if !sshMode(req.DAConn) {
		return rpc.DASizeList{}, fmt.Errorf("connect with SSH to measure the website and database size")
	}
	return MeasureSSH(req.DAConn, req.Sites)
}

func MeasureSSH(conn rpc.DAConn, sites []rpc.DASite) (rpc.DASizeList, error) {
	picked, args, err := sizeArgs(sites)
	if err != nil {
		return rpc.DASizeList{}, err
	}
	c, err := DialSSH(conn)
	if err != nil {
		return rpc.DASizeList{}, err
	}
	defer c.Close()
	if err := c.prepare(); err != nil {
		return rpc.DASizeList{}, err
	}
	var out bytes.Buffer
	if err := c.bashWithin(sshIdle, sshSizeMax, sizeSitesScript, args, &out); err != nil {
		return rpc.DASizeList{}, err
	}
	return ParseSiteSizes(out.String(), picked)
}

func sizeArgs(sites []rpc.DASite) ([]rpc.DASite, []string, error) {
	if len(sites) == 0 {
		return nil, nil, fmt.Errorf("select at least one website")
	}
	if len(sites) > 100 {
		return nil, nil, fmt.Errorf("select at most 100 websites")
	}
	seen := map[string]bool{}
	var picked []rpc.DASite
	var args []string
	for _, site := range sites {
		owner := strings.ToLower(strings.TrimSpace(site.Owner))
		if owner == "" || strings.ContainsAny(owner, " \t/\\.") || strings.ContainsAny(owner, "\"'`$") {
			return nil, nil, fmt.Errorf("%s has no DirectAdmin user", site.Domain)
		}
		domain, err := validate.ASCIIHost(site.Domain)
		if err != nil {
			return nil, nil, err
		}
		key := owner + "\n" + domain
		if seen[key] {
			continue
		}
		seen[key] = true
		picked = append(picked, rpc.DASite{Domain: domain, Owner: owner})
		args = append(args, owner, domain)
	}
	if len(picked) == 0 {
		return nil, nil, fmt.Errorf("select at least one website")
	}
	return picked, args, nil
}

func ParseSiteSizes(text string, want []rpc.DASite) (rpc.DASizeList, error) {
	found := map[string]rpc.DASiteSize{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 4 {
			continue
		}
		web, errWeb := strconv.ParseInt(parts[2], 10, 64)
		db, errDB := strconv.ParseInt(parts[3], 10, 64)
		if errWeb != nil || web < 0 || errDB != nil || db < -1 {
			continue
		}
		owner := strings.ToLower(strings.TrimSpace(parts[0]))
		domain := strings.ToLower(strings.TrimSpace(parts[1]))
		found[owner+"\n"+domain] = rpc.DASiteSize{Domain: domain, Owner: owner, WebBytes: web, DBBytes: db}
	}
	if len(found) == 0 {
		return rpc.DASizeList{}, fmt.Errorf("could not measure the website size")
	}
	rows := make([]rpc.DASiteSize, 0, len(want))
	for _, site := range want {
		row, ok := found[site.Owner+"\n"+site.Domain]
		if !ok {
			return rpc.DASizeList{}, fmt.Errorf("could not measure %s", site.Domain)
		}
		rows = append(rows, row)
	}
	var web int64
	dbByOwner := map[string]int64{}
	for _, row := range rows {
		web += row.WebBytes
		if _, ok := dbByOwner[row.Owner]; !ok {
			dbByOwner[row.Owner] = row.DBBytes
		}
	}
	db := int64(0)
	for _, n := range dbByOwner {
		if n < 0 {
			db = -1
			break
		}
		db += n
	}
	return rpc.DASizeList{Sites: rows, WebBytes: web, DBBytes: db}, nil
}

func PullSSH(conn rpc.DAConn, owner, domain, destDir string, progress func(string, int)) (string, error) {
	owner = strings.TrimSpace(owner)
	domain = strings.TrimSpace(domain)
	if owner == "" || domain == "" || strings.Contains(owner, "/") || strings.Contains(domain, "/") {
		return "", fmt.Errorf("website not found")
	}
	c, err := DialSSH(conn)
	if err != nil {
		return "", err
	}
	defer c.Close()
	if err := c.prepare(); err != nil {
		return "", err
	}
	if progress != nil {
		progress("Packing the website into a tar file", 20)
	}
	var packed bytes.Buffer
	if err := c.bashWithin(sshIdle, sshPackMax, copySiteScript, []string{owner, domain}, &packed); err != nil {
		return "", err
	}
	total, remote, err := parseReady(packed.String())
	if err != nil {
		return "", err
	}
	defer c.removeRemote(remote)
	if err := os.MkdirAll(destDir, 0750); err != nil {
		return "", err
	}
	dest := filepath.Join(destDir, "da-"+tarBase(remote))
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
	if err != nil {
		return "", err
	}
	if progress != nil {
		progress("Downloading the tar file", 40)
	}
	sniff := &sniffWriter{w: &limitWriter{w: f, max: maxArchiveBytes}}
	count := &countWriter{w: sniff, total: total, progress: func(pct int) {
		if progress != nil {
			progress("Downloading the tar file", pct)
		}
	}}
	cmd, stdin := c.sudoCmd("cat " + shellQuote(remote))
	err = c.execWithin(sshIdle, sshPullMax, cmd, stdin, count)
	closeErr := f.Close()
	if err != nil || sniff.bad || !sniff.checked || count.n == 0 {
		_ = os.Remove(dest)
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("SSH did not return the website archive")
	}
	if closeErr != nil {
		_ = os.Remove(dest)
		return "", closeErr
	}
	if progress != nil {
		progress("Downloading the tar file", 70)
	}
	return dest, nil
}

func tarBase(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

func parseReady(text string) (int64, string, error) {
	var size int64
	var path string
	found := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "ready ") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, "ready "))
		nText, file, ok := strings.Cut(rest, " ")
		if !ok {
			return 0, "", fmt.Errorf("SSH did not return the website archive")
		}
		n, err := strconv.ParseInt(nText, 10, 64)
		if err != nil || n <= 0 || n > maxArchiveBytes || !validRemoteTar(file) {
			return 0, "", fmt.Errorf("SSH did not return the website archive")
		}
		size = n
		path = file
		found = true
	}
	if !found {
		return 0, "", fmt.Errorf("SSH did not return the website archive")
	}
	return size, path, nil
}

func validRemoteTar(path string) bool {
	if len(path) > 400 || !strings.HasPrefix(path, "/") || strings.Contains(path, "..") || strings.ContainsAny(path, " \t\r\n'\"\\$;|&`") {
		return false
	}
	slash := strings.LastIndex(path, "/")
	dir := path[:slash]
	base := path[slash+1:]
	if dir != "/siroc-tmp" && !strings.HasSuffix(dir, "/siroc-tmp") {
		return false
	}
	if !strings.HasPrefix(base, "siroc-migrate-") || !strings.HasSuffix(base, ".tar.gz") {
		return false
	}
	for _, r := range base {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '.' && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

func (c *sshConn) sudoCmd(cmd string) (string, io.Reader) {
	if c.useSudoPass {
		return "sudo -S -p '' " + cmd, strings.NewReader(c.pass + "\n")
	}
	if c.useSudo {
		return "sudo -n " + cmd, nil
	}
	return cmd, nil
}

func (c *sshConn) removeRemote(path string) {
	cmd, stdin := c.sudoCmd("rm -f " + shellQuote(path))
	_ = c.exec(20*time.Second, cmd, stdin, nil)
}

func (c *sshConn) prepare() error {
	out, err := c.output(20*time.Second, "id -u")
	if err == nil && strings.TrimSpace(out) == "0" {
		return nil
	}
	out, err = c.output(20*time.Second, "sudo -n id -u")
	if err == nil && strings.TrimSpace(out) == "0" {
		c.useSudo = true
		return nil
	}
	if c.pass == "" {
		return fmt.Errorf("this SSH user is not root and sudo did not succeed")
	}
	var buf bytes.Buffer
	err = c.exec(20*time.Second, "sudo -S -p '' id -u", strings.NewReader(c.pass+"\n"), &buf)
	if err == nil && strings.TrimSpace(buf.String()) == "0" {
		c.useSudoPass = true
		return nil
	}
	return fmt.Errorf("this SSH user is not root and sudo did not succeed")
}

func (c *sshConn) output(timeout time.Duration, cmd string) (string, error) {
	var buf bytes.Buffer
	err := c.exec(timeout, cmd, nil, &buf)
	return buf.String(), err
}

func (c *sshConn) bash(timeout time.Duration, script string, args []string, stdout io.Writer) error {
	return c.bashWithin(timeout, timeout, script, args, stdout)
}

func (c *sshConn) bashWithin(idle, max time.Duration, script string, args []string, stdout io.Writer) error {
	cmd := "/bin/bash -s --"
	for _, arg := range args {
		cmd += " " + shellQuote(arg)
	}
	var stdin io.Reader = strings.NewReader(script)
	if c.useSudoPass {
		cmd = "sudo -S -p '' " + cmd
		stdin = io.MultiReader(strings.NewReader(c.pass+"\n"), strings.NewReader(script))
	} else if c.useSudo {
		cmd = "sudo -n " + cmd
	}
	return c.execWithin(idle, max, cmd, stdin, stdout)
}

func (c *sshConn) exec(timeout time.Duration, cmd string, stdin io.Reader, stdout io.Writer) error {
	return c.execWithin(timeout, timeout, cmd, stdin, stdout)
}

// execWithin stops a command that produces nothing for idle, and always stops at max.
// Output on stdout or stderr keeps a long pack or download alive.
func (c *sshConn) execWithin(idle, max time.Duration, cmd string, stdin io.Reader, stdout io.Writer) error {
	if idle <= 0 || idle > max {
		idle = max
	}
	session, err := c.cli.NewSession()
	if err != nil {
		return fmt.Errorf("SSH session failed")
	}
	defer session.Close()
	if stdout == nil {
		stdout = io.Discard
	}
	var stderr bytes.Buffer
	var last atomic.Int64
	touch := func() { last.Store(time.Now().UnixNano()) }
	touch()
	session.Stdin = stdin
	session.Stdout = touchWriter{w: stdout, touch: touch}
	session.Stderr = touchWriter{w: &stderr, touch: touch}
	var timedOut atomic.Bool
	var finished atomic.Bool
	stop := make(chan struct{})
	started := time.Now()
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if finished.Load() {
					return
				}
				quiet := time.Since(time.Unix(0, last.Load())) >= idle
				if time.Since(started) >= max || quiet {
					if finished.Load() {
						return
					}
					timedOut.Store(true)
					_ = c.cli.Close()
					return
				}
			}
		}
	}()
	err = session.Run(cmd)
	finished.Store(true)
	close(stop)
	if timedOut.Load() {
		return fmt.Errorf("SSH command timed out")
	}
	if err != nil {
		msg := clipSecret(stderr.String(), c.pass)
		if msg == "" {
			msg = "command failed"
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

type touchWriter struct {
	w     io.Writer
	touch func()
}

func (t touchWriter) Write(p []byte) (int, error) {
	if t.touch != nil && len(p) > 0 {
		t.touch()
	}
	if t.w == nil {
		return len(p), nil
	}
	return t.w.Write(p)
}

func clipSecret(msg, secret string) string {
	msg = strings.TrimSpace(msg)
	if secret != "" {
		msg = strings.ReplaceAll(msg, secret, "")
	}
	msg = strings.Join(strings.Fields(msg), " ")
	if len(msg) > 400 {
		msg = msg[:400]
	}
	return strings.TrimSpace(msg)
}

type sniffWriter struct {
	w       io.Writer
	buf     []byte
	bad     bool
	checked bool
}

func (s *sniffWriter) Write(p []byte) (int, error) {
	if s.bad {
		return len(p), nil
	}
	if !s.checked {
		s.buf = append(s.buf, p...)
		if len(s.buf) < 2 {
			return len(p), nil
		}
		s.checked = true
		if s.buf[0] != 0x1f || s.buf[1] != 0x8b {
			s.bad = true
			s.buf = nil
			return len(p), nil
		}
		if _, err := s.w.Write(s.buf); err != nil {
			return 0, err
		}
		s.buf = nil
		return len(p), nil
	}
	return s.w.Write(p)
}

type limitWriter struct {
	w   io.Writer
	n   int64
	max int64
}

func (l *limitWriter) Write(p []byte) (int, error) {
	if l.n+int64(len(p)) > l.max {
		return 0, fmt.Errorf("backup is larger than 20GB")
	}
	n, err := l.w.Write(p)
	l.n += int64(n)
	return n, err
}

type countWriter struct {
	w        io.Writer
	n        int64
	total    int64
	last     int
	progress func(int)
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	if c.progress != nil && c.total > 0 {
		pct := 40 + int(c.n*30/c.total)
		if pct > 70 {
			pct = 70
		}
		if pct > c.last {
			c.last = pct
			c.progress(pct)
		}
	}
	return n, err
}

const listSitesScript = `
set -eu
data=/usr/local/directadmin/data/users
if [ ! -d "$data" ]; then
  echo "DirectAdmin user data was not found" >&2
  exit 1
fi
for dir in "$data"/*; do
  [ -d "$dir" ] || continue
  user=$(basename "$dir")
  case "$user" in
    .*|*[!a-zA-Z0-9_-]*) continue ;;
  esac
  if [ -f "$dir/user.conf" ]; then
    dom=$(sed -n 's/^domain=//p' "$dir/user.conf" | head -n 1 | tr -d '\r')
    if [ -n "$dom" ]; then
      printf '%s\t%s\n' "$user" "$dom"
    fi
  fi
  if [ -d "$dir/domains" ]; then
    for conf in "$dir/domains/"*.conf; do
      [ -f "$conf" ] || continue
      base=$(basename "$conf" .conf)
      [ "$base" = "domain" ] && continue
      printf '%s\t%s\n' "$user" "$base"
    done
  fi
done
`

const sizeSitesScript = `
set -eu
if [ $# -lt 2 ]; then
  echo "select at least one website" >&2
  exit 1
fi
work=$(mktemp -d)
beatpid=
cleanup() {
  if [ -n "$beatpid" ]; then
    kill "$beatpid" 2>/dev/null || true
  fi
  rm -rf "$work"
}
trap cleanup EXIT
( while true; do sleep 30; printf 'beat\n'; done ) &
beatpid=$!
mysql_user=""
mysql_pass=""
mconf=/usr/local/directadmin/conf/mysql.conf
if [ -f "$mconf" ]; then
  while IFS= read -r line || [ -n "$line" ]; do
    line=${line%$'\r'}
    case "$line" in
      ""|\#*) continue ;;
    esac
    key=${line%%=*}
    val=${line#*=}
    key=$(printf '%s' "$key" | tr 'A-Z' 'a-z')
    case "$key" in
      user) [ -z "$mysql_user" ] && mysql_user=$val ;;
      passwd|password) [ -z "$mysql_pass" ] && mysql_pass=$val ;;
    esac
  done < "$mconf"
fi
cnf=""
if [ -n "$mysql_user" ]; then
  cnf="$work/client.cnf"
  umask 077
  {
    echo '[client]'
    printf 'user=%s\n' "$mysql_user"
    printf 'password=%s\n' "$mysql_pass"
  } > "$cnf"
fi
sqlbin=$(command -v mysql || true)
if [ -z "$sqlbin" ]; then
  sqlbin=$(command -v mariadb || true)
fi
sizes=$work/schema-sizes
: > "$sizes"
if [ -n "$sqlbin" ]; then
  if [ -n "$cnf" ]; then
    "$sqlbin" --defaults-extra-file="$cnf" -N -B -e "SELECT table_schema, IFNULL(SUM(DATA_LENGTH)+SUM(INDEX_LENGTH),0) FROM information_schema.TABLES GROUP BY table_schema" > "$sizes" 2>/dev/null || true
  else
    "$sqlbin" -N -B -e "SELECT table_schema, IFNULL(SUM(DATA_LENGTH)+SUM(INDEX_LENGTH),0) FROM information_schema.TABLES GROUP BY table_schema" > "$sizes" 2>/dev/null || true
  fi
fi
add_name() {
  n=$(printf '%s' "$1" | tr -d '\r')
  out=$2
  case "$n" in
    ""|*[!A-Za-z0-9_]*) return 0 ;;
    information_schema|performance_schema|mysql|sys) return 0 ;;
  esac
  grep -qxF -- "$n" "$out" 2>/dev/null || printf '%s\n' "$n" >> "$out"
}
db_for() {
  owner=$1
  cache=$work/db-$owner
  if [ -f "$cache" ]; then
    cat "$cache"
    return 0
  fi
  names=$work/names-$owner
  : > "$names"
  conf=/usr/local/directadmin/data/users/$owner/mysql.conf
  if [ -f "$conf" ]; then
    tr '&' '\n' < "$conf" > "$work/oneconf"
    while IFS= read -r line || [ -n "$line" ]; do
      line=${line%$'\r'}
      key=${line%%=*}
      val=${line#*=}
      key=$(printf '%s' "$key" | tr 'A-Z' 'a-z' | tr -d '[:space:]')
      case "$key" in
        user|dbuser|db|database|name) add_name "$val" "$names" ;;
      esac
    done < "$work/oneconf"
  fi
  list=/usr/local/directadmin/data/users/$owner/mysql.list
  if [ -f "$list" ]; then
    while IFS= read -r line || [ -n "$line" ]; do
      line=${line%$'\r'}
      line=${line%%[[:space:]]*}
      add_name "$line" "$names"
    done < "$list"
  fi
  if [ -n "$sqlbin" ]; then
    show=$work/show-$owner
    if [ -n "$cnf" ]; then
      "$sqlbin" --defaults-extra-file="$cnf" -N -B -e 'SHOW DATABASES' > "$show" 2>/dev/null || true
    else
      "$sqlbin" -N -B -e 'SHOW DATABASES' > "$show" 2>/dev/null || true
    fi
    if [ -f "$show" ]; then
      while IFS= read -r line || [ -n "$line" ]; do
        case "$line" in
          "${owner}_"*) add_name "$line" "$names" ;;
        esac
      done < "$show"
    fi
  fi
  if [ ! -s "$sizes" ]; then
    printf '%s\n' '-1' > "$cache"
  elif [ ! -s "$names" ]; then
    echo 0 > "$cache"
  else
    sum=0
    while IFS= read -r n || [ -n "$n" ]; do
      [ -n "$n" ] || continue
      got=$(awk -v n="$n" '$1==n {print $2; found=1; exit} END{if (!found) print ""}' "$sizes")
      case "$got" in
        ""|*[!0-9]*) ;;
        *) sum=$((sum + got)) ;;
      esac
    done < "$names"
    printf '%s\n' "$sum" > "$cache"
  fi
  cat "$cache"
}
while [ $# -ge 2 ]; do
  owner=$1
  domain=$2
  shift 2
  dir=/home/$owner/domains/$domain
  web=0
  if [ -d "$dir" ]; then
    web=$(du -sb --exclude=stats --exclude=logs --exclude=awstats "$dir" 2>/dev/null | awk 'NR==1 {print $1}') || web=0
  fi
  case "$web" in
    ""|*[!0-9]*) web=0 ;;
  esac
  db=$(db_for "$owner")
  case "$db" in
    ""|*[!0-9-]*) db=-1 ;;
  esac
  printf '%s\t%s\t%s\t%s\n' "$owner" "$domain" "$web" "$db"
done
`

const copySiteScript = `
set -eu
owner=$1
domain=$2
case "$owner" in
  ""|*[!a-zA-Z0-9_-]*) echo "website not found" >&2; exit 1 ;;
esac
case "$domain" in
  ""|*[!a-zA-Z0-9.-]*) echo "website not found" >&2; exit 1 ;;
esac
home="/home/$owner/domains/$domain"
if [ ! -d "$home" ]; then
  echo "website files were not found" >&2
  exit 1
fi
pick_root() {
  where=$(df -PT -B1 --output=avail,fstype,target 2>/dev/null | awk '
    NR==1 { next }
    {
      avail=$1+0; type=$2; target=$3
      for (i=4; i<=NF; i++) target=target " " $i
      if (type ~ /^(tmpfs|devtmpfs|squashfs|overlay|aufs|ramfs|proc|sysfs|cgroup|cgroup2|autofs|devpts|securityfs|pstore|bpf|tracefs|debugfs|fusectl|configfs|nsfs|efivarfs)$/) next
      if (target !~ /^\// || target ~ /[[:space:]]|\.\./) next
      if (avail > best) { best=avail; where=target }
    }
    END { printf "%s", where }
  ')
  if [ -z "$where" ]; then
    where=/var/tmp
  fi
  printf '%s' "$where"
}
root=$(pick_root)
case "$root" in
  /) workdir=/siroc-tmp ;;
  /*) workdir="${root%/}/siroc-tmp" ;;
  *) workdir=/var/tmp/siroc-tmp ;;
esac
mkdir -p "$workdir" 2>/dev/null || {
  workdir=/var/tmp/siroc-tmp
  mkdir -p "$workdir"
}
stage=$(mktemp -d "$workdir/stage.XXXXXX")
stamp=$(date -u +%Y%m%d-%H%M%S)
archive="$workdir/siroc-migrate-${domain}-${stamp}.tar.gz"
keep=0
beatpid=
cleanup() {
  if [ -n "$beatpid" ]; then
    kill "$beatpid" 2>/dev/null || true
  fi
  rm -rf "$stage"
  if [ "$keep" != 1 ]; then
    rm -f "$archive"
  fi
}
trap cleanup EXIT
( while true; do sleep 30; printf 'beat\n'; done ) &
beatpid=$!
mkdir -p "$stage/domains" "$stage/mysql"
printf 'username=%s\ndomain=%s\n' "$owner" "$domain" > "$stage/user.conf"
ln -s "$home" "$stage/domains/$domain"
data="/usr/local/directadmin/data/users/$owner/domains"
for kind in pointers subdomains; do
  src="$data/$domain.$kind"
  if [ -f "$src" ]; then
    cp -a "$src" "$stage/domains/$domain.$kind"
  fi
done
if [ -f "$data/$domain.conf" ]; then
  cp -a "$data/$domain.conf" "$stage/domains/$domain.conf"
fi
if [ -f "/usr/local/directadmin/data/users/$owner/ftp.passwd" ]; then
  cp -a "/usr/local/directadmin/data/users/$owner/ftp.passwd" "$stage/ftp.passwd"
fi
mysql_user=""
mysql_pass=""
mconf=/usr/local/directadmin/conf/mysql.conf
if [ -f "$mconf" ]; then
  while IFS= read -r line || [ -n "$line" ]; do
    line=${line%$'\r'}
    case "$line" in
      ""|\#*) continue ;;
    esac
    key=${line%%=*}
    val=${line#*=}
    key=$(printf '%s' "$key" | tr 'A-Z' 'a-z')
    case "$key" in
      user) [ -z "$mysql_user" ] && mysql_user=$val ;;
      passwd|password) [ -z "$mysql_pass" ] && mysql_pass=$val ;;
    esac
  done < "$mconf"
fi
cnf=""
if [ -n "$mysql_user" ]; then
  cnf="$stage/client.cnf"
  umask 077
  {
    echo '[client]'
    printf 'user=%s\n' "$mysql_user"
    printf 'password=%s\n' "$mysql_pass"
  } > "$cnf"
fi
dumpbin=$(command -v mysqldump || true)
if [ -z "$dumpbin" ]; then
  dumpbin=$(command -v mariadb-dump || true)
fi
mkdir -p "$stage/dbpass"
names="$stage/dblist"
: > "$names"
remember_db() {
  name=$1
  pass=$2
  case "$name" in
    ""|*[!A-Za-z0-9_]*) return 0 ;;
    information_schema|performance_schema|mysql|sys) return 0 ;;
  esac
  if grep -qxF -- "$name" "$names" 2>/dev/null; then
    return 0
  fi
  printf '%s\n' "$name" >> "$names"
  printf '%s' "$pass" > "$stage/dbpass/$name"
}
db_user=""
db_pass=""
db_name=""
finish_rec() {
  name=$db_name
  [ -n "$name" ] || name=$db_user
  remember_db "$name" "$db_pass"
  db_user=""
  db_pass=""
  db_name=""
}
take_conf() {
  file=$1
  [ -f "$file" ] || return 0
  tr '&' '\n' < "$file" > "$stage/oneconf"
  db_user=""
  db_pass=""
  db_name=""
  while IFS= read -r line || [ -n "$line" ]; do
    line=${line%$'\r'}
    case "$line" in
      ""|\#*) continue ;;
    esac
    key=${line%%=*}
    val=${line#*=}
    key=$(printf '%s' "$key" | tr 'A-Z' 'a-z' | tr -d '[:space:]')
    case "$key" in
      user|dbuser)
        if [ -n "$db_user$db_name" ]; then
          finish_rec
        fi
        db_user=$val
        ;;
      db|database|name) db_name=$val ;;
      passwd|password|pass) db_pass=$val ;;
    esac
  done < "$stage/oneconf"
  finish_rec
}
take_conf "/usr/local/directadmin/data/users/$owner/mysql.conf"
if [ -f "/usr/local/directadmin/data/users/$owner/mysql.list" ]; then
  while IFS= read -r line || [ -n "$line" ]; do
    line=${line%$'\r'}
    line=${line%%[[:space:]]*}
    remember_db "$line" ""
  done < "/usr/local/directadmin/data/users/$owner/mysql.list"
fi
mdir="/usr/local/directadmin/data/users/$owner/mysql"
if [ -d "$mdir" ]; then
  for file in "$mdir"/*.conf; do
    [ -f "$file" ] || continue
    take_conf "$file"
  done
fi
sqlbin=$(command -v mysql || true)
if [ -z "$sqlbin" ]; then
  sqlbin=$(command -v mariadb || true)
fi
if [ -n "$sqlbin" ]; then
  if [ -n "$cnf" ]; then
    "$sqlbin" --defaults-extra-file="$cnf" -N -B -e 'SHOW DATABASES' > "$stage/showdb" 2>/dev/null || true
  else
    "$sqlbin" -N -B -e 'SHOW DATABASES' > "$stage/showdb" 2>/dev/null || true
  fi
  if [ -f "$stage/showdb" ]; then
    while IFS= read -r line || [ -n "$line" ]; do
      case "$line" in
        "${owner}_"*) remember_db "$line" "" ;;
      esac
    done < "$stage/showdb"
  fi
fi
if [ -s "$names" ]; then
  if [ -z "$dumpbin" ]; then
    echo "mysqldump is not installed on the DirectAdmin server" >&2
    exit 1
  fi
  while IFS= read -r name || [ -n "$name" ]; do
    [ -n "$name" ] || continue
    out="$stage/mysql/$name.sql"
    errf="$stage/dberr"
    ok=0
    if [ -n "$cnf" ]; then
      if "$dumpbin" --defaults-extra-file="$cnf" --single-transaction --skip-lock-tables --no-tablespaces --result-file="$out" "$name" 2>"$errf"; then
        ok=1
      elif grep -qi 'unknown database' "$errf"; then
        rm -f "$out"
        continue
      elif "$dumpbin" --defaults-extra-file="$cnf" --single-transaction --skip-lock-tables --result-file="$out" "$name" 2>"$errf"; then
        ok=1
      fi
    else
      if "$dumpbin" --single-transaction --skip-lock-tables --no-tablespaces --result-file="$out" "$name" 2>"$errf"; then
        ok=1
      elif grep -qi 'unknown database' "$errf"; then
        rm -f "$out"
        continue
      elif "$dumpbin" --single-transaction --skip-lock-tables --result-file="$out" "$name" 2>"$errf"; then
        ok=1
      fi
    fi
    if [ "$ok" != 1 ] || [ ! -s "$out" ]; then
      echo "could not dump database $name" >&2
      cat "$errf" >&2 || true
      exit 1
    fi
    pass=$(cat "$stage/dbpass/$name" 2>/dev/null || true)
    {
      printf 'user=%s\n' "$name"
      printf 'passwd=%s\n' "$pass"
    } > "$stage/mysql/$name.conf"
  done < "$names"
fi
echo "packed databases: $(grep -c . "$names" 2>/dev/null || echo 0)" >&2
rm -rf "$stage/client.cnf" "$stage/dblist" "$stage/oneconf" "$stage/showdb" "$stage/dbpass" "$stage/dberr"
set +e
tar -C "$stage" -chzf "$archive" --exclude=stats --exclude=logs --exclude=awstats .
code=$?
set -e
if [ "$code" -gt 1 ]; then
  echo "could not pack the website" >&2
  exit "$code"
fi
if command -v gzip >/dev/null 2>&1; then
  gzip -t "$archive"
fi
size=$(wc -c < "$archive" | tr -d '[:space:]')
case "$size" in
  ""|*[!0-9]*) echo "could not pack the website" >&2; exit 1 ;;
esac
keep=1
printf 'ready %s %s\n' "$size" "$archive"
`

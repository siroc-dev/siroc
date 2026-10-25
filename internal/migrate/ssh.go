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
	return &sshConn{cli: cli, pass: conn.Password}, nil
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

func PullSSH(conn rpc.DAConn, owner, domain, destDir string, progress func(int)) (string, error) {
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
	var sizeOut bytes.Buffer
	_ = c.exec(30*time.Second, "du -sb "+shellQuote("/home/"+owner+"/domains/"+domain), nil, &sizeOut)
	total := parseDu(sizeOut.String())
	if err := os.MkdirAll(destDir, 0750); err != nil {
		return "", err
	}
	safe := strings.Map(func(ch rune) rune {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' {
			return ch
		}
		return '_'
	}, strings.ToLower(domain))
	dest := filepath.Join(destDir, "da-"+safe+"-"+time.Now().UTC().Format("20060102-150405")+".tar.gz")
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
	if err != nil {
		return "", err
	}
	sniff := &sniffWriter{w: &limitWriter{w: f, max: maxArchiveBytes}}
	count := &countWriter{w: sniff, total: total, progress: progress}
	err = c.bash(45*time.Minute, copySiteScript, []string{owner, domain}, count)
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
		progress(70)
	}
	return dest, nil
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
	return c.exec(timeout, cmd, stdin, stdout)
}

func (c *sshConn) exec(timeout time.Duration, cmd string, stdin io.Reader, stdout io.Writer) error {
	session, err := c.cli.NewSession()
	if err != nil {
		return fmt.Errorf("SSH session failed")
	}
	defer session.Close()
	if stdout == nil {
		stdout = io.Discard
	}
	var stderr bytes.Buffer
	session.Stdin = stdin
	session.Stdout = stdout
	session.Stderr = &stderr
	var timedOut atomic.Bool
	timer := time.AfterFunc(timeout, func() {
		timedOut.Store(true)
		_ = c.cli.Close()
	})
	defer timer.Stop()
	err = session.Run(cmd)
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

func parseDu(s string) int64 {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return 0
	}
	n, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
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
		pct := 20 + int(c.n*50/c.total)
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
stage=$(mktemp -d /tmp/siroc-migrate.XXXXXX)
trap 'rm -rf "$stage"' EXIT
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
list=/usr/local/directadmin/data/users/$owner/mysql.conf
db_user=""
db_pass=""
db_name=""
flush_db() {
  name=$db_name
  [ -n "$name" ] || name=$db_user
  pass_now=$db_pass
  db_user=""
  db_pass=""
  db_name=""
  case "$name" in
    ""|*[!A-Za-z0-9_]*) return 0 ;;
  esac
  if [ -z "$dumpbin" ]; then
    echo "mysqldump is not installed on the DirectAdmin server" >&2
    exit 1
  fi
  out="$stage/mysql/$name.sql"
  if [ -n "$cnf" ]; then
    "$dumpbin" --defaults-extra-file="$cnf" --single-transaction --skip-lock-tables --no-tablespaces --result-file="$out" "$name" \
      || "$dumpbin" --defaults-extra-file="$cnf" --single-transaction --skip-lock-tables --result-file="$out" "$name"
  else
    "$dumpbin" --single-transaction --skip-lock-tables --no-tablespaces --result-file="$out" "$name" \
      || "$dumpbin" --single-transaction --skip-lock-tables --result-file="$out" "$name"
  fi
  {
    printf 'user=%s\n' "$name"
    printf 'passwd=%s\n' "$pass_now"
  } > "$stage/mysql/$name.conf"
}
if [ -f "$list" ]; then
  tr '&' '\n' < "$list" > "$stage/dblist"
  while IFS= read -r line || [ -n "$line" ]; do
    line=${line%$'\r'}
    case "$line" in
      ""|\#*) continue ;;
    esac
    key=${line%%=*}
    val=${line#*=}
    key=$(printf '%s' "$key" | tr 'A-Z' 'a-z')
    case "$key" in
      user|dbuser)
        if [ -n "$db_user$db_name" ]; then
          flush_db
        fi
        db_user=$val
        ;;
      db) db_name=$val ;;
      passwd|password|pass) db_pass=$val ;;
    esac
  done < "$stage/dblist"
  flush_db
fi
rm -f "$stage/client.cnf" "$stage/dblist"
set +e
tar -C "$stage" -chzf - --exclude=stats --exclude=logs --exclude=awstats .
code=$?
set -e
if [ "$code" -gt 1 ]; then
  echo "could not pack the website" >&2
  exit "$code"
fi
`

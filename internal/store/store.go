package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	DB *sql.DB
}

type PanelUser struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         string
	Impersonator string
}

type Account struct {
	ID          int64     `json:"id"`
	Username    string    `json:"username"`
	LinuxUID    int       `json:"linuxUid"`
	LinuxGID    int       `json:"linuxGid"`
	Suspended   bool      `json:"suspended"`
	PHPCLI      string    `json:"phpCli"`
	PythonCLI   string    `json:"pythonCli"`
	NodeCLI     string    `json:"nodeCli"`
	SSHEnabled  bool      `json:"sshEnabled"`
	FTPEnabled  bool      `json:"ftpEnabled"`
	HasPassword bool      `json:"hasPassword"`
	PasswordEnc string    `json:"-"`
	PHPFpmJSON  string    `json:"-"`
	DiskQuotaMB int64     `json:"diskQuotaMB"`
	CreatedAt   time.Time `json:"createdAt"`
}

type FTPUser struct {
	ID          int64     `json:"id"`
	AccountID   int64     `json:"accountId"`
	Username    string    `json:"username"`
	Login       string    `json:"login"`
	Home        string    `json:"home"`
	HasPassword bool      `json:"hasPassword"`
	PasswordEnc string    `json:"-"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Site struct {
	ID         int64     `json:"id"`
	AccountID  int64     `json:"accountId"`
	Username   string    `json:"username"`
	Domain     string    `json:"domain"`
	DocRoot    string    `json:"docRoot"`
	PHPVersion string    `json:"phpVersion"`
	Enabled    bool      `json:"enabled"`
	Aliases    []string  `json:"aliases"`
	SSL        bool      `json:"ssl"`
	SSLKind    string    `json:"sslKind,omitempty"`
	SSLExpiry  string    `json:"sslExpiry,omitempty"`
	Rewrite    string    `json:"rewrite,omitempty"`
	Kind       string    `json:"kind,omitempty"`
	ProxyPass  string    `json:"proxyPass,omitempty"`
	AppPort    int       `json:"appPort,omitempty"`
	AppCmd     string    `json:"appCmd,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

type NginxRewrite struct {
	From string `json:"from"`
	To   string `json:"to"`
	Flag string `json:"flag"`
}

type Database struct {
	ID        int64     `json:"id"`
	AccountID int64     `json:"accountId"`
	Username  string    `json:"username"`
	DBName    string    `json:"dbName"`
	DBUser    string    `json:"dbUser"`
	Engine      string    `json:"engine"`
	HasPassword bool      `json:"hasPassword"`
	PasswordEnc string    `json:"-"`
	CreatedAt   time.Time `json:"createdAt"`
}

type BackupDest struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Kind        string    `json:"kind"`
	Host        string    `json:"host,omitempty"`
	Port        int       `json:"port,omitempty"`
	User        string    `json:"user,omitempty"`
	Path        string    `json:"path,omitempty"`
	Bucket      string    `json:"bucket,omitempty"`
	Region      string    `json:"region,omitempty"`
	Endpoint    string    `json:"endpoint,omitempty"`
	Prefix      string    `json:"prefix,omitempty"`
	UseSSL      bool      `json:"useSSL,omitempty"`
	HasSecret   bool      `json:"hasSecret"`
	PasswordEnc string    `json:"-"`
	SecretEnc   string    `json:"-"`
	CreatedAt   time.Time `json:"createdAt"`
}

type BackupJob struct {
	ID         int64     `json:"id"`
	Account    string    `json:"account"`
	DestID     int64     `json:"destId"`
	DestName   string    `json:"destName,omitempty"`
	Status     string    `json:"status"`
	Message    string    `json:"message,omitempty"`
	Size       int64     `json:"size"`
	LocalPath  string    `json:"localPath,omitempty"`
	Remote     string    `json:"remote,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	FinishedAt string    `json:"finishedAt,omitempty"`
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	s := &Store{DB: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) migrate() error {
	_, err := s.DB.Exec(`
CREATE TABLE IF NOT EXISTS panel_users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  username TEXT UNIQUE NOT NULL,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT 'admin',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS accounts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  username TEXT UNIQUE NOT NULL,
  linux_uid INTEGER NOT NULL,
  linux_gid INTEGER NOT NULL,
  suspended INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS sites (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  domain TEXT UNIQUE NOT NULL,
  docroot TEXT NOT NULL,
  php_version TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS databases (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  db_name TEXT UNIQUE NOT NULL,
  db_user TEXT NOT NULL,
  engine TEXT NOT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES panel_users(id) ON DELETE CASCADE,
  expires_at DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
`)
	if err != nil {
		return err
	}
	for _, col := range []string{
		`ALTER TABLE accounts ADD COLUMN php_cli TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE accounts ADD COLUMN python_cli TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE accounts ADD COLUMN node_cli TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE accounts ADD COLUMN password_enc TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE accounts ADD COLUMN ssh_enabled INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE accounts ADD COLUMN ftp_enabled INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE accounts ADD COLUMN php_fpm_json TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sessions ADD COLUMN impersonator_id INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE sites ADD COLUMN aliases_json TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE sites ADD COLUMN ssl_enabled INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE sites ADD COLUMN ssl_expiry TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sites ADD COLUMN nginx_rewrites_json TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE sites ADD COLUMN ssl_kind TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sites ADD COLUMN kind TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sites ADD COLUMN proxy_pass TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sites ADD COLUMN app_port INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE sites ADD COLUMN app_cmd TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE accounts ADD COLUMN disk_quota_mb INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE databases ADD COLUMN password_enc TEXT NOT NULL DEFAULT ''`,
	} {
		_, _ = s.DB.Exec(col)
	}
	_, _ = s.DB.Exec(`
CREATE TABLE IF NOT EXISTS ftp_users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  login TEXT UNIQUE NOT NULL,
  home TEXT NOT NULL,
  password_enc TEXT NOT NULL DEFAULT '',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS install_jobs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  version TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'queued',
  message TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  started_at DATETIME,
  finished_at DATETIME
);
CREATE TABLE IF NOT EXISTS backup_dests (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  kind TEXT NOT NULL,
  host TEXT NOT NULL DEFAULT '',
  port INTEGER NOT NULL DEFAULT 0,
  user_name TEXT NOT NULL DEFAULT '',
  path TEXT NOT NULL DEFAULT '',
  bucket TEXT NOT NULL DEFAULT '',
  region TEXT NOT NULL DEFAULT '',
  endpoint TEXT NOT NULL DEFAULT '',
  prefix TEXT NOT NULL DEFAULT '',
  use_ssl INTEGER NOT NULL DEFAULT 1,
  password_enc TEXT NOT NULL DEFAULT '',
  secret_enc TEXT NOT NULL DEFAULT '',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS backup_jobs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  account TEXT NOT NULL,
  dest_id INTEGER NOT NULL DEFAULT 0,
  dest_name TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'queued',
  message TEXT NOT NULL DEFAULT '',
  size INTEGER NOT NULL DEFAULT 0,
  local_path TEXT NOT NULL DEFAULT '',
  remote TEXT NOT NULL DEFAULT '',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  finished_at TEXT NOT NULL DEFAULT ''
);`)
	return s.ensureSecret()
}

func (s *Store) ensureSecret() error {
	v, err := s.Setting("secret_key")
	if err != nil {
		return err
	}
	if v != "" {
		return nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	return s.SetSetting("secret_key", hex.EncodeToString(b))
}

func (s *Store) SecretKey() (string, error) {
	if err := s.ensureSecret(); err != nil {
		return "", err
	}
	return s.Setting("secret_key")
}

func (s *Store) PanelUserCount() (int, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM panel_users`).Scan(&n)
	return n, err
}

func (s *Store) CreatePanelUser(username, hash, role string) error {
	_, err := s.DB.Exec(`INSERT INTO panel_users (username, password_hash, role) VALUES (?, ?, ?)`, username, hash, role)
	return err
}

func (s *Store) GetPanelUserByName(username string) (*PanelUser, error) {
	u := &PanelUser{}
	err := s.DB.QueryRow(`SELECT id, username, password_hash, role FROM panel_users WHERE username = ?`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Store) GetPanelUser(id int64) (*PanelUser, error) {
	u := &PanelUser{}
	err := s.DB.QueryRow(`SELECT id, username, password_hash, role FROM panel_users WHERE id = ?`, id).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Store) UpdatePassword(id int64, hash string) error {
	_, err := s.DB.Exec(`UPDATE panel_users SET password_hash = ? WHERE id = ?`, hash, id)
	return err
}

func (s *Store) CreateSession(userID, impersonatorID int64, ttl time.Duration) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := hex.EncodeToString(b)
	exp := time.Now().Add(ttl).UTC().Format(time.RFC3339)
	_, err := s.DB.Exec(`INSERT INTO sessions (id, user_id, expires_at, impersonator_id) VALUES (?, ?, ?, ?)`, id, userID, exp, impersonatorID)
	return id, err
}

func (s *Store) GetSessionUser(sid string) (*PanelUser, error) {
	var userID, impersonatorID int64
	var expStr string
	err := s.DB.QueryRow(`SELECT user_id, expires_at, impersonator_id FROM sessions WHERE id = ?`, sid).Scan(&userID, &expStr, &impersonatorID)
	if err != nil {
		return nil, err
	}
	exp, err := time.Parse(time.RFC3339, expStr)
	if err != nil {
		return nil, err
	}
	if time.Now().After(exp) {
		s.DB.Exec(`DELETE FROM sessions WHERE id = ?`, sid)
		return nil, sql.ErrNoRows
	}
	u, err := s.GetPanelUser(userID)
	if err != nil {
		return nil, err
	}
	if impersonatorID > 0 {
		if admin, err := s.GetPanelUser(impersonatorID); err == nil {
			u.Impersonator = admin.Username
		}
	}
	return u, nil
}

func (s *Store) DeleteSession(sid string) error {
	_, err := s.DB.Exec(`DELETE FROM sessions WHERE id = ?`, sid)
	return err
}

func (s *Store) CreateAccount(username string, uid, gid int, passwordEnc string, ssh, ftp bool) (*Account, error) {
	sshV, ftpV := 0, 0
	if ssh {
		sshV = 1
	}
	if ftp {
		ftpV = 1
	}
	res, err := s.DB.Exec(`INSERT INTO accounts (username, linux_uid, linux_gid, password_enc, ssh_enabled, ftp_enabled) VALUES (?, ?, ?, ?, ?, ?)`, username, uid, gid, passwordEnc, sshV, ftpV)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetAccount(id)
}

const accountCols = `id, username, linux_uid, linux_gid, suspended, php_cli, python_cli, node_cli, password_enc, ssh_enabled, ftp_enabled, php_fpm_json, disk_quota_mb, created_at`

func scanAccount(scan func(dest ...any) error) (*Account, error) {
	a := &Account{}
	var sus, ssh, ftp int
	var created string
	if err := scan(&a.ID, &a.Username, &a.LinuxUID, &a.LinuxGID, &sus, &a.PHPCLI, &a.PythonCLI, &a.NodeCLI, &a.PasswordEnc, &ssh, &ftp, &a.PHPFpmJSON, &a.DiskQuotaMB, &created); err != nil {
		return nil, err
	}
	a.Suspended = sus == 1
	a.SSHEnabled = ssh == 1
	a.FTPEnabled = ftp == 1
	a.HasPassword = a.PasswordEnc != ""
	a.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
	return a, nil
}

func (s *Store) GetAccount(id int64) (*Account, error) {
	return scanAccount(func(dest ...any) error {
		return s.DB.QueryRow(`SELECT `+accountCols+` FROM accounts WHERE id = ?`, id).Scan(dest...)
	})
}

func (s *Store) GetAccountByName(username string) (*Account, error) {
	var id int64
	err := s.DB.QueryRow(`SELECT id FROM accounts WHERE username = ?`, username).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.GetAccount(id)
}

func (s *Store) ListAccounts() ([]Account, error) {
	rows, err := s.DB.Query(`SELECT ` + accountCols + ` FROM accounts ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		a, err := scanAccount(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	if out == nil {
		out = []Account{}
	}
	return out, rows.Err()
}

func (s *Store) SetAccountSuspended(username string, suspended bool) error {
	v := 0
	if suspended {
		v = 1
	}
	_, err := s.DB.Exec(`UPDATE accounts SET suspended = ? WHERE username = ?`, v, username)
	return err
}

func (s *Store) UpdateAccountCLI(username, php, python, node string) error {
	_, err := s.DB.Exec(`UPDATE accounts SET php_cli = ?, python_cli = ?, node_cli = ? WHERE username = ?`, php, python, node, username)
	return err
}

func (s *Store) UpdateAccountPassword(username, passwordEnc string) error {
	_, err := s.DB.Exec(`UPDATE accounts SET password_enc = ? WHERE username = ?`, passwordEnc, username)
	return err
}

func (s *Store) UpdateAccountQuota(username string, limitMB int64) error {
	_, err := s.DB.Exec(`UPDATE accounts SET disk_quota_mb = ? WHERE username = ?`, limitMB, username)
	return err
}

func (s *Store) UpdateAccountAccess(username string, ssh, ftp bool) error {
	sshV, ftpV := 0, 0
	if ssh {
		sshV = 1
	}
	if ftp {
		ftpV = 1
	}
	_, err := s.DB.Exec(`UPDATE accounts SET ssh_enabled = ?, ftp_enabled = ? WHERE username = ?`, sshV, ftpV, username)
	return err
}

func (s *Store) UpdateAccountPHP(username, json string) error {
	_, err := s.DB.Exec(`UPDATE accounts SET php_fpm_json = ? WHERE username = ?`, json, username)
	return err
}

func (s *Store) DeleteAccount(username string) error {
	_, err := s.DB.Exec(`DELETE FROM accounts WHERE username = ?`, username)
	return err
}

func (s *Store) DeleteHostPanelUser(username string) error {
	_, err := s.DB.Exec(`DELETE FROM panel_users WHERE username = ? AND role = 'user'`, username)
	return err
}

func (s *Store) UpsertHostPanelUser(username, hash string) error {
	existing, err := s.GetPanelUserByName(username)
	if err != nil {
		return s.CreatePanelUser(username, hash, "user")
	}
	if existing.Role == "admin" {
		return fmt.Errorf("username %q is a panel administrator", username)
	}
	return s.UpdatePassword(existing.ID, hash)
}

func (s *Store) CreateFTPUser(accountID int64, login, home, passwordEnc string) (*FTPUser, error) {
	res, err := s.DB.Exec(`INSERT INTO ftp_users (account_id, login, home, password_enc) VALUES (?, ?, ?, ?)`, accountID, login, home, passwordEnc)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetFTPUser(id)
}

func (s *Store) GetFTPUser(id int64) (*FTPUser, error) {
	return scanFTP(func(dest ...any) error {
		return s.DB.QueryRow(`
SELECT f.id, f.account_id, a.username, f.login, f.home, f.password_enc, f.created_at
FROM ftp_users f JOIN accounts a ON a.id = f.account_id WHERE f.id = ?`, id).Scan(dest...)
	})
}

func (s *Store) GetFTPUserByLogin(login string) (*FTPUser, error) {
	var id int64
	err := s.DB.QueryRow(`SELECT id FROM ftp_users WHERE login = ?`, login).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.GetFTPUser(id)
}

func scanFTP(scan func(dest ...any) error) (*FTPUser, error) {
	f := &FTPUser{}
	var created string
	if err := scan(&f.ID, &f.AccountID, &f.Username, &f.Login, &f.Home, &f.PasswordEnc, &created); err != nil {
		return nil, err
	}
	f.HasPassword = f.PasswordEnc != ""
	f.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
	return f, nil
}

func (s *Store) ListFTPUsers(accountID int64) ([]FTPUser, error) {
	q := `
SELECT f.id, f.account_id, a.username, f.login, f.home, f.password_enc, f.created_at
FROM ftp_users f JOIN accounts a ON a.id = f.account_id`
	args := []any{}
	if accountID > 0 {
		q += ` WHERE f.account_id = ?`
		args = append(args, accountID)
	}
	q += ` ORDER BY f.login`
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FTPUser
	for rows.Next() {
		f, err := scanFTP(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *f)
	}
	if out == nil {
		out = []FTPUser{}
	}
	return out, rows.Err()
}

func (s *Store) UpdateFTPPassword(id int64, passwordEnc string) error {
	_, err := s.DB.Exec(`UPDATE ftp_users SET password_enc = ? WHERE id = ?`, passwordEnc, id)
	return err
}

func (s *Store) DeleteFTPUser(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM ftp_users WHERE id = ?`, id)
	return err
}

func (s *Store) CountSites(accountID int64) (int, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM sites WHERE account_id = ?`, accountID).Scan(&n)
	return n, err
}

func (s *Store) CountDatabases(accountID int64) (int, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM databases WHERE account_id = ?`, accountID).Scan(&n)
	return n, err
}

func (s *Store) CreateSite(accountID int64, domain, docroot, php string, aliases []string, kind, proxyPass, rewrite string, appPort int, appCmd string) (*Site, error) {
	raw, err := json.Marshal(aliasesOrEmpty(aliases))
	if err != nil {
		return nil, err
	}
	if kind == "" {
		kind = "php"
	}
	rw, err := json.Marshal(rewrite)
	if err != nil {
		return nil, err
	}
	res, err := s.DB.Exec(`INSERT INTO sites (account_id, domain, docroot, php_version, aliases_json, ssl_enabled, ssl_kind, kind, proxy_pass, nginx_rewrites_json, app_port, app_cmd) VALUES (?, ?, ?, ?, ?, 1, 'local', ?, ?, ?, ?, ?)`, accountID, domain, docroot, php, string(raw), kind, proxyPass, string(rw), appPort, appCmd)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetSite(id)
}

func (s *Store) GetSite(id int64) (*Site, error) {
	st := &Site{}
	var en, ssl int
	var created, aliases, rewrites string
	err := s.DB.QueryRow(`
SELECT s.id, s.account_id, a.username, s.domain, s.docroot, s.php_version, s.enabled, s.aliases_json, s.ssl_enabled, s.ssl_expiry, s.ssl_kind, s.nginx_rewrites_json, s.kind, s.proxy_pass, s.app_port, s.app_cmd, s.created_at
FROM sites s JOIN accounts a ON a.id = s.account_id WHERE s.id = ?`, id).
		Scan(&st.ID, &st.AccountID, &st.Username, &st.Domain, &st.DocRoot, &st.PHPVersion, &en, &aliases, &ssl, &st.SSLExpiry, &st.SSLKind, &rewrites, &st.Kind, &st.ProxyPass, &st.AppPort, &st.AppCmd, &created)
	if err != nil {
		return nil, err
	}
	st.Enabled = en == 1
	st.SSL = ssl == 1
	st.Aliases = parseAliases(aliases)
	st.Rewrite = parseRewriteText(rewrites)
	st.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
	return st, nil
}

func (s *Store) GetSiteByDomain(domain string) (*Site, error) {
	var id int64
	err := s.DB.QueryRow(`SELECT id FROM sites WHERE domain = ?`, domain).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.GetSite(id)
}

func (s *Store) ListSites() ([]Site, error) {
	rows, err := s.DB.Query(`
SELECT s.id, s.account_id, a.username, s.domain, s.docroot, s.php_version, s.enabled, s.aliases_json, s.ssl_enabled, s.ssl_expiry, s.ssl_kind, s.nginx_rewrites_json, s.kind, s.proxy_pass, s.app_port, s.app_cmd, s.created_at
FROM sites s JOIN accounts a ON a.id = s.account_id ORDER BY s.domain`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Site
	for rows.Next() {
		var st Site
		var en, ssl int
		var created, aliases, rewrites string
		if err := rows.Scan(&st.ID, &st.AccountID, &st.Username, &st.Domain, &st.DocRoot, &st.PHPVersion, &en, &aliases, &ssl, &st.SSLExpiry, &st.SSLKind, &rewrites, &st.Kind, &st.ProxyPass, &st.AppPort, &st.AppCmd, &created); err != nil {
			return nil, err
		}
		st.Enabled = en == 1
		st.SSL = ssl == 1
		st.Aliases = parseAliases(aliases)
		st.Rewrite = parseRewriteText(rewrites)
		st.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
		out = append(out, st)
	}
	if out == nil {
		out = []Site{}
	}
	return out, rows.Err()
}

func (s *Store) UpdateSite(id int64, php, docroot string, enabled bool, aliases []string, ssl bool, expiry, kind, rewrite string) error {
	v, sv := 0, 0
	if enabled {
		v = 1
	}
	if ssl {
		sv = 1
	}
	if !ssl {
		kind = ""
	}
	raw, err := json.Marshal(aliasesOrEmpty(aliases))
	if err != nil {
		return err
	}
	rw, err := json.Marshal(rewrite)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`UPDATE sites SET php_version = ?, docroot = ?, enabled = ?, aliases_json = ?, ssl_enabled = ?, ssl_expiry = ?, ssl_kind = ?, nginx_rewrites_json = ? WHERE id = ?`, php, docroot, v, string(raw), sv, expiry, kind, string(rw), id)
	return err
}

func (s *Store) RenameSite(id int64, domain, php, docroot string, enabled bool, aliases []string, ssl bool, expiry, kind, rewrite string) error {
	if err := s.UpdateSite(id, php, docroot, enabled, aliases, ssl, expiry, kind, rewrite); err != nil {
		return err
	}
	_, err := s.DB.Exec(`UPDATE sites SET domain = ? WHERE id = ?`, domain, id)
	return err
}

func (s *Store) UpdateSiteProxy(id int64, kind, proxyPass string) error {
	return s.UpdateSiteApp(id, kind, proxyPass, 0, "")
}

func (s *Store) UpdateSiteApp(id int64, kind, proxyPass string, appPort int, appCmd string) error {
	if kind == "" {
		kind = "php"
	}
	_, err := s.DB.Exec(`UPDATE sites SET kind = ?, proxy_pass = ?, app_port = ?, app_cmd = ? WHERE id = ?`, kind, proxyPass, appPort, appCmd, id)
	return err
}

func (s *Store) AppPortTaken(port int, exceptID int64) (bool, error) {
	if port <= 0 {
		return false, nil
	}
	list, err := s.ListSites()
	if err != nil {
		return false, err
	}
	for _, st := range list {
		if st.ID == exceptID {
			continue
		}
		if st.AppPort == port {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) NextAppPort(exceptID int64) (int, error) {
	for p := 30000; p <= 39999; p++ {
		taken, err := s.AppPortTaken(p, exceptID)
		if err != nil {
			return 0, err
		}
		if !taken {
			return p, nil
		}
	}
	return 0, fmt.Errorf("no free app port in 30000-39999")
}

func (s *Store) HostnameTaken(name string, exceptID int64) (bool, error) {
	list, err := s.ListSites()
	if err != nil {
		return false, err
	}
	name = strings.ToLower(strings.TrimSpace(name))
	for _, st := range list {
		if st.ID == exceptID {
			continue
		}
		if st.Domain == name {
			return true, nil
		}
		for _, a := range st.Aliases {
			if a == name {
				return true, nil
			}
		}
	}
	return false, nil
}

func parseAliases(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil || out == nil {
		return []string{}
	}
	return out
}

func aliasesOrEmpty(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func parseRewriteText(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" || raw == "null" {
		return ""
	}
	var arr []NginxRewrite
	if json.Unmarshal([]byte(raw), &arr) == nil {
		var b strings.Builder
		for _, r := range arr {
			from := strings.TrimSpace(r.From)
			to := strings.TrimSpace(r.To)
			if from == "" {
				continue
			}
			flag := strings.TrimSpace(r.Flag)
			if flag == "" {
				flag = "last"
			}
			fmt.Fprintf(&b, "rewrite %s %s %s;\n", from, to, flag)
		}
		return strings.TrimSpace(b.String())
	}
	var s string
	if json.Unmarshal([]byte(raw), &s) == nil {
		return strings.TrimSpace(s)
	}
	return raw
}

func (s *Store) DeleteSite(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM sites WHERE id = ?`, id)
	return err
}

func (s *Store) CreateDatabase(accountID int64, name, user, engine, passwordEnc string) (*Database, error) {
	res, err := s.DB.Exec(`INSERT INTO databases (account_id, db_name, db_user, engine, password_enc) VALUES (?, ?, ?, ?, ?)`, accountID, name, user, engine, passwordEnc)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetDatabase(id)
}

func (s *Store) GetDatabase(id int64) (*Database, error) {
	d := &Database{}
	var created string
	err := s.DB.QueryRow(`
SELECT d.id, d.account_id, a.username, d.db_name, d.db_user, d.engine, d.password_enc, d.created_at
FROM databases d JOIN accounts a ON a.id = d.account_id WHERE d.id = ?`, id).
		Scan(&d.ID, &d.AccountID, &d.Username, &d.DBName, &d.DBUser, &d.Engine, &d.PasswordEnc, &created)
	if err != nil {
		return nil, err
	}
	d.HasPassword = d.PasswordEnc != ""
	d.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
	return d, nil
}

func (s *Store) UpdateDatabasePassword(id int64, passwordEnc string) error {
	_, err := s.DB.Exec(`UPDATE databases SET password_enc = ? WHERE id = ?`, passwordEnc, id)
	return err
}

func (s *Store) ListDatabases() ([]Database, error) {
	rows, err := s.DB.Query(`
SELECT d.id, d.account_id, a.username, d.db_name, d.db_user, d.engine, d.password_enc, d.created_at
FROM databases d JOIN accounts a ON a.id = d.account_id ORDER BY d.db_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Database
	for rows.Next() {
		var d Database
		var created string
		if err := rows.Scan(&d.ID, &d.AccountID, &d.Username, &d.DBName, &d.DBUser, &d.Engine, &d.PasswordEnc, &created); err != nil {
			return nil, err
		}
		d.HasPassword = d.PasswordEnc != ""
		d.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
		out = append(out, d)
	}
	if out == nil {
		out = []Database{}
	}
	return out, rows.Err()
}

func (s *Store) DeleteDatabase(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM databases WHERE id = ?`, id)
	return err
}

func (s *Store) Counts() (accounts, sites, databases int, err error) {
	err = s.DB.QueryRow(`SELECT
		(SELECT COUNT(*) FROM accounts),
		(SELECT COUNT(*) FROM sites),
		(SELECT COUNT(*) FROM databases)`).Scan(&accounts, &sites, &databases)
	return
}

func (s *Store) Setting(key string) (string, error) {
	var v string
	err := s.DB.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.DB.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func SanitizeName(s string) error {
	if len(s) < 3 || len(s) > 32 {
		return fmt.Errorf("name must be 3-32 characters")
	}
	return nil
}

type InstallJob struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Version    string `json:"version"`
	Status     string `json:"status"`
	Message    string `json:"message"`
	CreatedAt  string `json:"createdAt"`
	StartedAt  string `json:"startedAt,omitempty"`
	FinishedAt string `json:"finishedAt,omitempty"`
}

func (s *Store) EnqueueInstall(name, version string) (*InstallJob, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.DB.Exec(`INSERT INTO install_jobs (name, version, status, message, created_at) VALUES (?, ?, 'queued', '', ?)`, name, version, now)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetInstallJob(id)
}

func (s *Store) GetInstallJob(id int64) (*InstallJob, error) {
	j := &InstallJob{}
	var started, finished sql.NullString
	err := s.DB.QueryRow(`SELECT id, name, version, status, message, created_at, started_at, finished_at FROM install_jobs WHERE id = ?`, id).
		Scan(&j.ID, &j.Name, &j.Version, &j.Status, &j.Message, &j.CreatedAt, &started, &finished)
	if err != nil {
		return nil, err
	}
	j.StartedAt = started.String
	j.FinishedAt = finished.String
	return j, nil
}

func (s *Store) ListInstallJobs(limit int) ([]InstallJob, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.DB.Query(`SELECT id, name, version, status, message, created_at, started_at, finished_at FROM install_jobs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InstallJob
	for rows.Next() {
		var j InstallJob
		var started, finished sql.NullString
		if err := rows.Scan(&j.ID, &j.Name, &j.Version, &j.Status, &j.Message, &j.CreatedAt, &started, &finished); err != nil {
			return nil, err
		}
		j.StartedAt = started.String
		j.FinishedAt = finished.String
		out = append(out, j)
	}
	if out == nil {
		out = []InstallJob{}
	}
	return out, rows.Err()
}

func (s *Store) ActiveInstalls() ([]InstallJob, error) {
	rows, err := s.DB.Query(`SELECT id, name, version, status, message, created_at, started_at, finished_at FROM install_jobs WHERE status IN ('queued', 'running') ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InstallJob
	for rows.Next() {
		var j InstallJob
		var started, finished sql.NullString
		if err := rows.Scan(&j.ID, &j.Name, &j.Version, &j.Status, &j.Message, &j.CreatedAt, &started, &finished); err != nil {
			return nil, err
		}
		j.StartedAt = started.String
		j.FinishedAt = finished.String
		out = append(out, j)
	}
	if out == nil {
		out = []InstallJob{}
	}
	return out, rows.Err()
}

func (s *Store) HasActiveInstall(name, version string) (bool, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM install_jobs WHERE name = ? AND version = ? AND status IN ('queued', 'running')`, name, version).Scan(&n)
	return n > 0, err
}

func (s *Store) NextQueuedInstall() (*InstallJob, error) {
	j := &InstallJob{}
	var started, finished sql.NullString
	err := s.DB.QueryRow(`SELECT id, name, version, status, message, created_at, started_at, finished_at FROM install_jobs WHERE status = 'queued' ORDER BY id LIMIT 1`).
		Scan(&j.ID, &j.Name, &j.Version, &j.Status, &j.Message, &j.CreatedAt, &started, &finished)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	j.StartedAt = started.String
	j.FinishedAt = finished.String
	return j, nil
}

func (s *Store) SetInstallRunning(id int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.Exec(`UPDATE install_jobs SET status = 'running', started_at = ?, message = '' WHERE id = ? AND status = 'queued'`, now, id)
	return err
}

func (s *Store) FinishInstall(id int64, status, message string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.Exec(`UPDATE install_jobs SET status = ?, message = ?, finished_at = ? WHERE id = ?`, status, message, now, id)
	return err
}

func (s *Store) CancelInstall(id int64) error {
	res, err := s.DB.Exec(`UPDATE install_jobs SET status = 'canceled', message = 'canceled', finished_at = ? WHERE id = ? AND status = 'queued'`, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("job is not queued")
	}
	return nil
}

func (s *Store) RequeueInterruptedInstalls() error {
	_, err := s.DB.Exec(`UPDATE install_jobs SET status = 'queued', started_at = NULL, message = 'resumed after restart' WHERE status = 'running'`)
	return err
}

func (s *Store) ClearFinishedInstalls() (int64, error) {
	res, err := s.DB.Exec(`DELETE FROM install_jobs WHERE status NOT IN ('queued', 'running')`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

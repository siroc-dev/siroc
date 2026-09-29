package store

import (
	"database/sql"
	"strings"
	"time"
)

type SiteGit struct {
	SiteID   int64  `json:"siteId"`
	Repo     string `json:"repo"`
	Branch   string `json:"branch"`
	Path     string `json:"path"`
	Command  string `json:"command"`
	Token    string `json:"token"`
	LastOK   bool   `json:"lastOk"`
	LastLog  string `json:"lastLog"`
	LastAt   string `json:"lastAt,omitempty"`
}

func (s *Store) ensureSiteGitTable() {
	_, _ = s.DB.Exec(`
CREATE TABLE IF NOT EXISTS site_git (
  site_id INTEGER PRIMARY KEY REFERENCES sites(id) ON DELETE CASCADE,
  repo TEXT NOT NULL DEFAULT '',
  branch TEXT NOT NULL DEFAULT 'main',
  path TEXT NOT NULL DEFAULT '',
  command TEXT NOT NULL DEFAULT '',
  token TEXT UNIQUE NOT NULL,
  last_ok INTEGER NOT NULL DEFAULT 0,
  last_log TEXT NOT NULL DEFAULT '',
  last_at TEXT NOT NULL DEFAULT ''
);`)
}

func scanSiteGit(scan func(dest ...any) error) (*SiteGit, error) {
	g := &SiteGit{}
	var ok int
	if err := scan(&g.SiteID, &g.Repo, &g.Branch, &g.Path, &g.Command, &g.Token, &ok, &g.LastLog, &g.LastAt); err != nil {
		return nil, err
	}
	g.LastOK = ok == 1
	return g, nil
}

func (s *Store) GetSiteGit(siteID int64) (*SiteGit, error) {
	s.ensureSiteGitTable()
	return scanSiteGit(func(dest ...any) error {
		return s.DB.QueryRow(`SELECT site_id, repo, branch, path, command, token, last_ok, last_log, last_at FROM site_git WHERE site_id = ?`, siteID).Scan(dest...)
	})
}

func (s *Store) GetSiteGitByToken(token string) (*SiteGit, error) {
	s.ensureSiteGitTable()
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, sql.ErrNoRows
	}
	return scanSiteGit(func(dest ...any) error {
		return s.DB.QueryRow(`SELECT site_id, repo, branch, path, command, token, last_ok, last_log, last_at FROM site_git WHERE token = ?`, token).Scan(dest...)
	})
}

func (s *Store) UpsertSiteGit(g *SiteGit) error {
	s.ensureSiteGitTable()
	if g.Branch == "" {
		g.Branch = "main"
	}
	_, err := s.DB.Exec(`
INSERT INTO site_git (site_id, repo, branch, path, command, token, last_ok, last_log, last_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(site_id) DO UPDATE SET
  repo = excluded.repo,
  branch = excluded.branch,
  path = excluded.path,
  command = excluded.command,
  token = excluded.token`,
		g.SiteID, g.Repo, g.Branch, g.Path, g.Command, g.Token, boolInt(g.LastOK), g.LastLog, g.LastAt)
	return err
}

func (s *Store) UpdateSiteGitResult(siteID int64, ok bool, log string) error {
	s.ensureSiteGitTable()
	if len(log) > 64<<10 {
		log = log[len(log)-64<<10:]
	}
	_, err := s.DB.Exec(`UPDATE site_git SET last_ok = ?, last_log = ?, last_at = ? WHERE site_id = ?`,
		boolInt(ok), log, time.Now().UTC().Format(time.RFC3339), siteID)
	return err
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

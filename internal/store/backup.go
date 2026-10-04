package store

import (
	"database/sql"
	"time"
)

func (s *Store) CreateBackupDest(d BackupDest) (*BackupDest, error) {
	ssl := 0
	if d.UseSSL {
		ssl = 1
	}
	res, err := s.DB.Exec(`INSERT INTO backup_dests (name, kind, host, port, user_name, path, bucket, region, endpoint, prefix, use_ssl, password_enc, secret_enc) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.Name, d.Kind, d.Host, d.Port, d.User, d.Path, d.Bucket, d.Region, d.Endpoint, d.Prefix, ssl, d.PasswordEnc, d.SecretEnc)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetBackupDest(id)
}

func (s *Store) GetBackupDest(id int64) (*BackupDest, error) {
	d := &BackupDest{}
	var ssl int
	var created string
	err := s.DB.QueryRow(`SELECT id, name, kind, host, port, user_name, path, bucket, region, endpoint, prefix, use_ssl, password_enc, secret_enc, created_at FROM backup_dests WHERE id = ?`, id).
		Scan(&d.ID, &d.Name, &d.Kind, &d.Host, &d.Port, &d.User, &d.Path, &d.Bucket, &d.Region, &d.Endpoint, &d.Prefix, &ssl, &d.PasswordEnc, &d.SecretEnc, &created)
	if err != nil {
		return nil, err
	}
	d.UseSSL = ssl == 1
	d.HasSecret = d.PasswordEnc != "" || d.SecretEnc != ""
	d.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
	return d, nil
}

func (s *Store) ListBackupDests() ([]BackupDest, error) {
	rows, err := s.DB.Query(`SELECT id, name, kind, host, port, user_name, path, bucket, region, endpoint, prefix, use_ssl, password_enc, secret_enc, created_at FROM backup_dests ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BackupDest
	for rows.Next() {
		var d BackupDest
		var ssl int
		var created string
		if err := rows.Scan(&d.ID, &d.Name, &d.Kind, &d.Host, &d.Port, &d.User, &d.Path, &d.Bucket, &d.Region, &d.Endpoint, &d.Prefix, &ssl, &d.PasswordEnc, &d.SecretEnc, &created); err != nil {
			return nil, err
		}
		d.UseSSL = ssl == 1
		d.HasSecret = d.PasswordEnc != "" || d.SecretEnc != ""
		d.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
		out = append(out, d)
	}
	if out == nil {
		out = []BackupDest{}
	}
	return out, rows.Err()
}

func (s *Store) CountBackupCronsByDest(destID int64) (int, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM backup_crons WHERE dest_id = ?`, destID).Scan(&n)
	return n, err
}

func (s *Store) DeleteBackupDest(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM backup_dests WHERE id = ?`, id)
	return err
}

func (s *Store) CreateBackupJob(account string, destID int64, destName string) (*BackupJob, error) {
	res, err := s.DB.Exec(`INSERT INTO backup_jobs (account, dest_id, dest_name, status) VALUES (?, ?, ?, 'running')`, account, destID, destName)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetBackupJob(id)
}

func (s *Store) GetBackupJob(id int64) (*BackupJob, error) {
	j := &BackupJob{}
	var created string
	err := s.DB.QueryRow(`SELECT id, account, dest_id, dest_name, status, message, size, local_path, remote, created_at, finished_at FROM backup_jobs WHERE id = ?`, id).
		Scan(&j.ID, &j.Account, &j.DestID, &j.DestName, &j.Status, &j.Message, &j.Size, &j.LocalPath, &j.Remote, &created, &j.FinishedAt)
	if err != nil {
		return nil, err
	}
	j.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
	return j, nil
}

func (s *Store) FinishBackupJob(id int64, status, message, localPath, remote string, size int64) error {
	_, err := s.DB.Exec(`UPDATE backup_jobs SET status = ?, message = ?, local_path = ?, remote = ?, size = ?, finished_at = CURRENT_TIMESTAMP WHERE id = ?`, status, message, localPath, remote, size, id)
	return err
}

func (s *Store) ListBackupJobs(account string, limit int) ([]BackupJob, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT id, account, dest_id, dest_name, status, message, size, local_path, remote, created_at, finished_at FROM backup_jobs`
	args := []any{}
	if account != "" {
		q += ` WHERE account = ?`
		args = append(args, account)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BackupJob
	for rows.Next() {
		var j BackupJob
		var created string
		if err := rows.Scan(&j.ID, &j.Account, &j.DestID, &j.DestName, &j.Status, &j.Message, &j.Size, &j.LocalPath, &j.Remote, &created, &j.FinishedAt); err != nil {
			return nil, err
		}
		j.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
		out = append(out, j)
	}
	if out == nil {
		out = []BackupJob{}
	}
	return out, rows.Err()
}

func (s *Store) CreateBackupCron(c BackupCron) (*BackupCron, error) {
	inc, en := 0, 0
	if c.IncludeDB {
		inc = 1
	}
	if c.Enabled {
		en = 1
	}
	res, err := s.DB.Exec(`INSERT INTO backup_crons (name, account, dest_id, include_db, cycle, minute, hour, weekday, monthday, retain, enabled) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Name, c.Account, c.DestID, inc, c.Cycle, c.Minute, c.Hour, c.Weekday, c.Monthday, c.Retain, en)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetBackupCron(id)
}

func (s *Store) GetBackupCron(id int64) (*BackupCron, error) {
	list, err := s.ListBackupCrons()
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == id {
			return &list[i], nil
		}
	}
	return nil, sql.ErrNoRows
}

func (s *Store) ListBackupCrons() ([]BackupCron, error) {
	rows, err := s.DB.Query(`SELECT c.id, c.name, c.account, c.dest_id, COALESCE(d.name, ''), c.include_db, c.cycle, c.minute, c.hour, c.weekday, c.monthday, c.retain, c.enabled, c.last_run, c.last_status, c.created_at FROM backup_crons c LEFT JOIN backup_dests d ON d.id = c.dest_id ORDER BY c.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BackupCron
	for rows.Next() {
		var c BackupCron
		var inc, en int
		var created string
		if err := rows.Scan(&c.ID, &c.Name, &c.Account, &c.DestID, &c.DestName, &inc, &c.Cycle, &c.Minute, &c.Hour, &c.Weekday, &c.Monthday, &c.Retain, &en, &c.LastRun, &c.LastStatus, &created); err != nil {
			return nil, err
		}
		c.IncludeDB = inc == 1
		c.Enabled = en == 1
		c.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
		out = append(out, c)
	}
	if out == nil {
		out = []BackupCron{}
	}
	return out, rows.Err()
}

func (s *Store) DeleteBackupCron(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM backup_crons WHERE id = ?`, id)
	return err
}

func (s *Store) TouchBackupCron(id int64, status, message string) error {
	if len(message) > 400 {
		message = message[:400]
	}
	text := status
	if message != "" {
		text = status + ": " + message
	}
	_, err := s.DB.Exec(`UPDATE backup_crons SET last_run = CURRENT_TIMESTAMP, last_status = ? WHERE id = ?`, text, id)
	return err
}

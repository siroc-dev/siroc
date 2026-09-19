package store

import (
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

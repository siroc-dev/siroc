package api

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/secret"
	"github.com/siroc-dev/siroc/internal/store"
	"github.com/siroc-dev/siroc/internal/validate"
)

func (s *Server) fetchFile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		User string `json:"user"`
		Path string `json:"path"`
		URL  string `json:"url"`
		Dest string `json:"dest"`
	}
	if !decode(w, r, &body) {
		return
	}
	username, root, ok := s.fileUser(w, r, body.User)
	if !ok {
		return
	}
	out, err := s.Agent.FileFetch(rpc.FileReq{Username: username, Path: body.Path, URL: body.URL, Dest: body.Dest, Root: root})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) searchFiles(w http.ResponseWriter, r *http.Request) {
	var body struct {
		User  string `json:"user"`
		Path  string `json:"path"`
		Query string `json:"query"`
	}
	if !decode(w, r, &body) {
		return
	}
	username, root, ok := s.fileUser(w, r, body.User)
	if !ok {
		return
	}
	out, err := s.Agent.FileSearch(rpc.FileReq{Username: username, Path: body.Path, Query: body.Query, Root: root})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) listBackupDests(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListBackupDests()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createBackupDest(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(currentUser(r)) {
		writeErr(w, http.StatusForbidden, fmt.Errorf("admin only"))
		return
	}
	var body struct {
		Name      string `json:"name"`
		Kind      string `json:"kind"`
		Host      string `json:"host"`
		Port      int    `json:"port"`
		User      string `json:"user"`
		Password  string `json:"password"`
		Path      string `json:"path"`
		Bucket    string `json:"bucket"`
		Region    string `json:"region"`
		Endpoint  string `json:"endpoint"`
		Prefix    string `json:"prefix"`
		UseSSL    *bool  `json:"useSSL"`
		AccessKey string `json:"accessKey"`
		SecretKey string `json:"secretKey"`
	}
	if !decode(w, r, &body) {
		return
	}
	kind := strings.ToLower(strings.TrimSpace(body.Kind))
	switch kind {
	case "local", "ftp", "s3":
	default:
		writeErr(w, http.StatusBadRequest, fmt.Errorf("kind must be local, ftp, or s3"))
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("name required"))
		return
	}
	d := store.BackupDest{
		Name:     name,
		Kind:     kind,
		Host:     strings.TrimSpace(body.Host),
		Port:     body.Port,
		User:     strings.TrimSpace(body.User),
		Path:     strings.TrimSpace(body.Path),
		Bucket:   strings.TrimSpace(body.Bucket),
		Region:   strings.TrimSpace(body.Region),
		Endpoint: strings.TrimSpace(body.Endpoint),
		Prefix:   strings.TrimSpace(body.Prefix),
		UseSSL:   true,
	}
	if body.UseSSL != nil {
		d.UseSSL = *body.UseSSL
	}
	if kind == "s3" && body.AccessKey != "" {
		d.User = strings.TrimSpace(body.AccessKey)
	}
	if body.Password != "" {
		enc, err := s.encryptPassword(body.Password)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		d.PasswordEnc = enc
	}
	secretVal := body.SecretKey
	if secretVal == "" && kind == "s3" {
		secretVal = body.Password
	}
	if secretVal != "" {
		enc, err := s.encryptPassword(secretVal)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		d.SecretEnc = enc
	}
	out, err := s.Store.CreateBackupDest(d)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) sysopsStatus(w http.ResponseWriter, _ *http.Request) {
	out, err := s.Agent.Sysops(rpc.SysopsReq{Action: "status"})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) sysopsApply(w http.ResponseWriter, r *http.Request) {
	var body rpc.SysopsReq
	if !decode(w, r, &body) {
		return
	}
	out, err := s.Agent.Sysops(body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) sysopsDisk(w http.ResponseWriter, _ *http.Request) {
	out, err := s.Agent.Disk()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) setAccountQuota(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(currentUser(r)) {
		writeErr(w, http.StatusForbidden, fmt.Errorf("admin only"))
		return
	}
	username := chi.URLParam(r, "username")
	if err := validate.LinuxUser(username); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var body struct {
		LimitMB int64 `json:"limitMB"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.LimitMB < 0 {
		body.LimitMB = 0
	}
	out, err := s.Agent.QuotaSet(username, body.LimitMB)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = s.Store.UpdateAccountQuota(username, body.LimitMB)
	if out != nil {
		out.LimitMB = body.LimitMB
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) listAccountRedis(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(currentUser(r)) {
		writeErr(w, http.StatusForbidden, fmt.Errorf("admin only"))
		return
	}
	out, err := s.Agent.ListUserRedis()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if out == nil {
		out = []rpc.UserRedis{}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getAccountRedis(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	if _, ok := s.accountOrErr(w, r, username); !ok {
		return
	}
	if err := validate.LinuxUser(username); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	out, err := s.Agent.UserRedis(username)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) setAccountRedis(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(currentUser(r)) {
		writeErr(w, http.StatusForbidden, fmt.Errorf("admin only"))
		return
	}
	username := chi.URLParam(r, "username")
	if _, ok := s.accountOrErr(w, r, username); !ok {
		return
	}
	if err := validate.LinuxUser(username); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var body struct {
		Enabled        *bool `json:"enabled"`
		RotatePassword bool  `json:"rotatePassword"`
	}
	if !decode(w, r, &body) {
		return
	}
	out, err := s.Agent.SetUserRedis(rpc.UserRedisReq{
		Username:       username,
		Enabled:        body.Enabled,
		RotatePassword: body.RotatePassword,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) listQuota(w http.ResponseWriter, r *http.Request) {
	accs, err := s.Store.ListAccounts()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	u := currentUser(r)
	var out []rpc.QuotaInfo
	for _, a := range accs {
		if !isAdmin(u) && u.Username != a.Username {
			continue
		}
		info, err := s.Agent.QuotaGet(a.Username)
		if err != nil {
			info = &rpc.QuotaInfo{Username: a.Username, LimitMB: a.DiskQuotaMB, Message: err.Error()}
		} else {
			info.LimitMB = a.DiskQuotaMB
			if a.DiskQuotaMB > 0 {
				info.LimitBytes = a.DiskQuotaMB * 1024 * 1024
			}
		}
		out = append(out, *info)
	}
	if out == nil {
		out = []rpc.QuotaInfo{}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) pmaAutologin(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid id"))
		return
	}
	db, err := s.Store.GetDatabase(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, fmt.Errorf("database not found"))
		return
	}
	if !s.allowAccount(w, r, db.Username) {
		return
	}
	pass := ""
	if db.PasswordEnc != "" {
		pass, err = s.decryptPassword(db.PasswordEnc)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
	} else {
		pass, err = secret.RandomPassword(16)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		if err := s.Agent.DBPassword(db.DBUser, pass); err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("reset database password for phpMyAdmin: %w", err))
			return
		}
		if enc, e := s.encryptPassword(pass); e == nil {
			_ = s.Store.UpdateDatabasePassword(db.ID, enc)
		}
	}
	out, err := s.Agent.PMASignon(rpc.PMASignonReq{DBUser: db.DBUser, Password: pass, Host: "127.0.0.1"})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func rewritePMACookiePath(c string) string {
	parts := strings.Split(c, ";")
	found := false
	for i, p := range parts {
		trim := strings.TrimSpace(p)
		key, val, ok := strings.Cut(trim, "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "Path") {
			continue
		}
		found = true
		cur := strings.TrimSpace(val)
		if cur == "/" || cur == "" {
			parts[i] = " Path=/pma/"
		}
	}
	out := strings.Join(parts, ";")
	if !found {
		out += "; Path=/pma/"
	}
	return out
}

func (s *Server) pmaProxy() http.Handler {
	target, _ := url.Parse("http://127.0.0.1:9088")
	proxy := httputil.NewSingleHostReverseProxy(target)
	orig := proxy.Director
	proxy.Director = func(req *http.Request) {
		orig(req)
		req.URL.Path = strings.TrimPrefix(req.URL.Path, "/pma")
		if req.URL.Path == "" {
			req.URL.Path = "/"
		}
		req.Host = "pma.cp.local"
		req.Header.Set("X-Forwarded-Prefix", "/pma")
	}
	proxy.ModifyResponse = func(res *http.Response) error {
		if loc := res.Header.Get("Location"); loc != "" {
			if strings.HasPrefix(loc, "/") && !strings.HasPrefix(loc, "/pma") {
				res.Header.Set("Location", "/pma"+loc)
			}
		}
		if cookies := res.Header.Values("Set-Cookie"); len(cookies) > 0 {
			res.Header.Del("Set-Cookie")
			for _, c := range cookies {
				res.Header.Add("Set-Cookie", rewritePMACookiePath(c))
			}
		}
		return nil
	}
	return proxy
}

func (s *Server) listBackupJobs(w http.ResponseWriter, r *http.Request) {
	account := ""
	u := currentUser(r)
	if !isAdmin(u) {
		account = u.Username
	} else if q := r.URL.Query().Get("account"); q != "" {
		account = q
	}
	list, err := s.Store.ListBackupJobs(account, 80)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) deleteBackupDest(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(currentUser(r)) {
		writeErr(w, http.StatusForbidden, fmt.Errorf("admin only"))
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid id"))
		return
	}
	if err := s.Store.DeleteBackupDest(id); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) runBackup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username  string `json:"username"`
		DestID    int64  `json:"destId"`
		IncludeDB bool   `json:"includeDB"`
	}
	if !decode(w, r, &body) {
		return
	}
	acc, ok := s.accountOrErr(w, r, body.Username)
	if !ok {
		return
	}
	dest, err := s.Store.GetBackupDest(body.DestID)
	if err != nil {
		writeErr(w, http.StatusNotFound, fmt.Errorf("destination not found"))
		return
	}
	job, err := s.Store.CreateBackupJob(acc.Username, dest.ID, dest.Name)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	req := rpc.BackupReq{
		Username:  acc.Username,
		Kind:      dest.Kind,
		LocalDir:  dest.Path,
		Host:      dest.Host,
		Port:      dest.Port,
		User:      dest.User,
		Path:      dest.Path,
		Endpoint:  dest.Endpoint,
		Region:    dest.Region,
		Bucket:    dest.Bucket,
		Prefix:    dest.Prefix,
		UseSSL:    dest.UseSSL,
		IncludeDB: body.IncludeDB,
	}
	if dest.PasswordEnc != "" {
		req.Password, _ = s.decryptPassword(dest.PasswordEnc)
	}
	if dest.SecretEnc != "" {
		req.SecretKey, _ = s.decryptPassword(dest.SecretEnc)
	}
	req.AccessKey = dest.User
	if dest.Kind == "s3" && dest.User != "" {
		req.AccessKey = dest.User
	}
	if body.IncludeDB {
		dbs, _ := s.Store.ListDatabases()
		for _, d := range dbs {
			if d.Username == acc.Username {
				req.Databases = append(req.Databases, d.DBName)
			}
		}
	}
	out, err := s.Agent.BackupRun(req)
	if err != nil {
		_ = s.Store.FinishBackupJob(job.ID, "error", err.Error(), "", "", 0)
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = s.Store.FinishBackupJob(job.ID, "ok", out.Message, out.Path, out.Remote, out.Size)
	job, _ = s.Store.GetBackupJob(job.ID)
	writeJSON(w, http.StatusOK, map[string]any{"job": job, "result": out})
}

func (s *Server) restoreBackup(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(currentUser(r)) {
		writeErr(w, http.StatusForbidden, fmt.Errorf("admin only"))
		return
	}
	var body struct {
		Username string `json:"username"`
		Path     string `json:"path"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !s.allowAccount(w, r, body.Username) {
		return
	}
	out, err := s.Agent.BackupRun(rpc.BackupReq{Username: body.Username, Restore: body.Path})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) installWordPress(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid id"))
		return
	}
	st, err := s.Store.GetSite(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, fmt.Errorf("site not found"))
		return
	}
	if !s.allowAccount(w, r, st.Username) {
		return
	}
	var body struct {
		Title      string `json:"title"`
		URL        string `json:"url"`
		AdminUser  string `json:"adminUser"`
		AdminPass  string `json:"adminPass"`
		AdminEmail string `json:"adminEmail"`
		DBSuffix   string `json:"dbSuffix"`
		Prefix     string `json:"prefix"`
	}
	if !decode(w, r, &body) {
		return
	}
	if len(strings.TrimSpace(body.AdminPass)) < 8 {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("admin password must be at least 8 characters"))
		return
	}
	acc, ok := s.accountOrErr(w, r, st.Username)
	if !ok {
		return
	}
	engine, err := s.Agent.DBEngine()
	if err != nil || engine == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("install MySQL or MariaDB first"))
		return
	}
	suf := strings.TrimSpace(body.DBSuffix)
	if suf == "" {
		suf = "wp"
	}
	if err := validate.DBIdent(suf); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	dbPass, err := secret.RandomPassword(16)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	dbName := acc.Username + "_" + suf
	dbUser := acc.Username + "_" + suf
	if err := s.Agent.DBCreate(rpc.DBCreateReq{DBName: dbName, DBUser: dbUser, Password: dbPass, Engine: engine}); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	enc, _ := s.encryptPassword(dbPass)
	if _, err := s.Store.CreateDatabase(acc.ID, dbName, dbUser, engine, enc); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	siteURL := strings.TrimSpace(body.URL)
	if siteURL == "" {
		siteURL = "http://" + st.Domain
	}
	out, err := s.Agent.SiteApp(rpc.SiteAppReq{
		Username:   st.Username,
		Domain:     st.Domain,
		DocRoot:    st.DocRoot,
		PHPVersion: st.PHPVersion,
		Action:     "install-wordpress",
		Name:       body.Title,
		URL:        siteURL,
		AdminUser:  body.AdminUser,
		AdminPass:  body.AdminPass,
		AdminEmail: body.AdminEmail,
		DBName:     dbName,
		DBUser:     dbUser,
		DBPass:     dbPass,
		Prefix:     strings.TrimSpace(body.Prefix),
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

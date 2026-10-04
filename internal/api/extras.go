package api

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/siroc-dev/siroc/internal/auth"
	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/secret"
	"github.com/siroc-dev/siroc/internal/siteopts"
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
		if req.TLS != nil && req.Header.Get("X-Forwarded-Proto") == "" {
			req.Header.Set("X-Forwarded-Proto", "https")
		}
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
	var healMu sync.Mutex
	var healed time.Time
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		healMu.Lock()
		if s.Agent != nil && time.Since(healed) > 15*time.Second {
			_ = s.Agent.PMAEnsure()
			healed = time.Now()
		}
		healMu.Unlock()
		http.Error(w, "phpMyAdmin backend is down. Install PHP and phpMyAdmin from Software, then open it again from Databases. ("+err.Error()+")", http.StatusBadGateway)
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
		IncludeDB *bool  `json:"includeDB"`
	}
	if !decode(w, r, &body) {
		return
	}
	includeDB := true
	if body.IncludeDB != nil {
		includeDB = *body.IncludeDB
	}
	dest, err := s.Store.GetBackupDest(body.DestID)
	if err != nil {
		writeErr(w, http.StatusNotFound, fmt.Errorf("destination not found"))
		return
	}
	names, ok := s.backupUsernames(w, r, body.Username)
	if !ok {
		return
	}
	var jobs []*store.BackupJob
	var last *rpc.BackupResp
	var firstErr error
	for _, name := range names {
		acc, err := s.Store.GetAccountByName(name)
		if err != nil {
			continue
		}
		job, err := s.Store.CreateBackupJob(acc.Username, dest.ID, dest.Name)
		if err != nil {
			firstErr = err
			continue
		}
		req := s.backupDestReq(dest)
		req.Username = acc.Username
		req.IncludeDB = includeDB
		req.Manifest = s.userBackupManifest(acc)
		for _, d := range req.Manifest.Databases {
			req.Databases = append(req.Databases, d.DBName)
		}
		out, err := s.Agent.BackupRun(req)
		if err != nil {
			_ = s.Store.FinishBackupJob(job.ID, "error", err.Error(), "", "", 0)
			if firstErr == nil {
				firstErr = err
			}
			job, _ = s.Store.GetBackupJob(job.ID)
			jobs = append(jobs, job)
			continue
		}
		_ = s.Store.FinishBackupJob(job.ID, "ok", out.Message, out.Path, out.Remote, out.Size)
		job, _ = s.Store.GetBackupJob(job.ID)
		jobs = append(jobs, job)
		last = out
	}
	if firstErr != nil && last == nil {
		writeErr(w, http.StatusBadRequest, firstErr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs, "result": last})
}

func (s *Server) backupUsernames(w http.ResponseWriter, r *http.Request, raw string) ([]string, bool) {
	name := strings.TrimSpace(raw)
	if name == "" || name == "*" {
		if !isAdmin(currentUser(r)) {
			writeErr(w, http.StatusForbidden, fmt.Errorf("admin only"))
			return nil, false
		}
		list, err := s.Store.ListAccounts()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return nil, false
		}
		var out []string
		for _, a := range list {
			out = append(out, a.Username)
		}
		if len(out) == 0 {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("no hosting accounts to back up"))
			return nil, false
		}
		return out, true
	}
	if _, ok := s.accountOrErr(w, r, name); !ok {
		return nil, false
	}
	return []string{name}, true
}

func (s *Server) backupDestReq(dest *store.BackupDest) rpc.BackupReq {
	req := rpc.BackupReq{
		Kind:     dest.Kind,
		LocalDir: dest.Path,
		Host:     dest.Host,
		Port:     dest.Port,
		User:     dest.User,
		Path:     dest.Path,
		Endpoint: dest.Endpoint,
		Region:   dest.Region,
		Bucket:   dest.Bucket,
		Prefix:   dest.Prefix,
		UseSSL:   dest.UseSSL,
	}
	if dest.PasswordEnc != "" {
		req.Password, _ = s.decryptPassword(dest.PasswordEnc)
	}
	if dest.SecretEnc != "" {
		req.SecretKey, _ = s.decryptPassword(dest.SecretEnc)
	}
	req.AccessKey = dest.User
	return req
}

func (s *Server) inspectBackup(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(currentUser(r)) {
		writeErr(w, http.StatusForbidden, fmt.Errorf("admin only"))
		return
	}
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	out, err := s.Agent.BackupRun(rpc.BackupReq{Restore: path, Inspect: true})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
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
	out, err := s.Agent.BackupRun(rpc.BackupReq{Username: strings.TrimSpace(body.Username), Restore: body.Path})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if out != nil && out.Manifest != nil {
		if err := s.applyBackupManifest(out.Manifest); err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("files restored, but panel records failed: %w", err))
			return
		}
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
		generated, err := secret.RandomIdent(6)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		suf = generated
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

func (s *Server) userBackupManifest(acc *store.Account) *rpc.BackupManifest {
	man := &rpc.BackupManifest{
		Version:  1,
		Username: acc.Username,
		Created:  time.Now().UTC().Format(time.RFC3339),
		Account: rpc.BackupAccount{
			Username:    acc.Username,
			SSH:         acc.SSHEnabled,
			FTP:         acc.FTPEnabled,
			PHPCLI:      acc.PHPCLI,
			PythonCLI:   acc.PythonCLI,
			NodeCLI:     acc.NodeCLI,
			DiskQuotaMB: acc.DiskQuotaMB,
			WAFEnabled:  acc.WAFEnabled,
			PHPFpmJSON:  acc.PHPFpmJSON,
		},
	}
	if acc.PasswordEnc != "" {
		if pw, err := s.decryptPassword(acc.PasswordEnc); err == nil {
			man.Account.Password = pw
		}
	}
	sites, _ := s.Store.ListSites()
	for _, st := range sites {
		if st.Username != acc.Username {
			continue
		}
		row := rpc.BackupSite{
			Domain:     st.Domain,
			DocRoot:    st.DocRoot,
			PHPVersion: st.PHPVersion,
			Enabled:    st.Enabled,
			Aliases:    st.Aliases,
			SSL:        st.SSL,
			SSLKind:    st.SSLKind,
			Rewrite:    st.Rewrite,
			Kind:       st.Kind,
			ProxyPass:  st.ProxyPass,
			AppPort:    st.AppPort,
			AppCmd:     st.AppCmd,
			WAF:        st.WAFEnabled,
			WAFRemove:  append([]int(nil), st.WAFDisabledIDs...),
		}
		if !siteopts.IsZero(st.Options) {
			opt := st.Options
			row.Options = &opt
		}
		if g, err := s.Store.GetSiteGit(st.ID); err == nil && g != nil {
			row.GitRepo = g.Repo
			row.GitBranch = g.Branch
			row.GitPath = g.Path
			row.GitCommand = g.Command
			row.GitToken = g.Token
		}
		man.Sites = append(man.Sites, row)
	}
	dbs, _ := s.Store.ListDatabases()
	for _, d := range dbs {
		if d.Username != acc.Username {
			continue
		}
		row := rpc.BackupDatabase{DBName: d.DBName, DBUser: d.DBUser, Engine: d.Engine}
		if d.PasswordEnc != "" {
			if pw, err := s.decryptPassword(d.PasswordEnc); err == nil {
				row.Password = pw
			}
		}
		man.Databases = append(man.Databases, row)
	}
	ftps, _ := s.Store.ListFTPUsers(acc.ID)
	for _, f := range ftps {
		row := rpc.BackupFTP{Login: f.Login, Home: f.Home}
		if f.PasswordEnc != "" {
			if pw, err := s.decryptPassword(f.PasswordEnc); err == nil {
				row.Password = pw
			}
		}
		man.FTP = append(man.FTP, row)
	}
	return man
}

func (s *Server) applyBackupManifest(man *rpc.BackupManifest) error {
	if man == nil {
		return nil
	}
	username := strings.TrimSpace(man.Username)
	if username == "" {
		username = strings.TrimSpace(man.Account.Username)
	}
	if err := validate.LinuxUser(username); err != nil {
		return err
	}
	pass := strings.TrimSpace(man.Account.Password)
	if pass == "" {
		pw, err := secret.RandomPassword(16)
		if err != nil {
			return err
		}
		pass = pw
		man.Account.Password = pass
	}
	u, err := s.Agent.UserEnsure(rpc.UserEnsureReq{Username: username, Password: pass})
	if err != nil {
		return err
	}
	_ = s.Agent.UserAccess(username, man.Account.SSH, man.Account.FTP)
	enc, err := s.encryptPassword(pass)
	if err != nil {
		return err
	}
	hash, err := auth.Hash(pass)
	if err != nil {
		return err
	}
	if err := s.Store.UpsertHostPanelUser(username, hash); err != nil {
		return err
	}
	acc, err := s.Store.GetAccountByName(username)
	if err != nil {
		acc, err = s.Store.CreateAccount(username, u.UID, u.GID, enc, man.Account.SSH, man.Account.FTP)
		if err != nil {
			return err
		}
	} else {
		_ = s.Store.UpdateAccountPassword(username, enc)
		_ = s.Store.UpdateAccountAccess(username, man.Account.SSH, man.Account.FTP)
	}
	if man.Account.PHPCLI != "" || man.Account.PythonCLI != "" || man.Account.NodeCLI != "" {
		_ = s.Store.UpdateAccountCLI(username, man.Account.PHPCLI, man.Account.PythonCLI, man.Account.NodeCLI)
	}
	if man.Account.DiskQuotaMB > 0 {
		_ = s.Store.UpdateAccountQuota(username, man.Account.DiskQuotaMB)
	}
	_ = s.Store.UpdateAccountWAF(username, man.Account.WAFEnabled)
	if man.Account.PHPFpmJSON != "" {
		_ = s.Store.UpdateAccountPHP(username, man.Account.PHPFpmJSON)
	}
	for _, d := range man.Databases {
		if _, err := s.Store.GetDatabaseByName(d.DBName); err == nil {
			continue
		}
		dbEnc := ""
		if d.Password != "" {
			dbEnc, _ = s.encryptPassword(d.Password)
		}
		_, _ = s.Store.CreateDatabase(acc.ID, d.DBName, d.DBUser, d.Engine, dbEnc)
	}
	for _, st := range man.Sites {
		if existing, err := s.Store.GetSiteByDomain(st.Domain); err == nil {
			if existing.Username != username {
				continue
			}
			_ = s.writeRestoredSite(acc, st)
			_ = s.restoreSiteOptions(existing.ID, st.Options)
			if st.WAFRemove != nil {
				_ = s.Store.UpdateSiteWAFRules(existing.ID, st.WAFRemove)
			}
			if st.GitRepo != "" {
				_ = s.Store.UpsertSiteGit(&store.SiteGit{
					SiteID: existing.ID, Repo: st.GitRepo, Branch: st.GitBranch, Path: st.GitPath, Command: st.GitCommand, Token: st.GitToken,
				})
			}
			continue
		}
		created, err := s.Store.CreateSite(acc.ID, st.Domain, st.DocRoot, st.PHPVersion, st.Aliases, st.Kind, st.ProxyPass, st.Rewrite, st.AppPort, st.AppCmd)
		if err != nil {
			return err
		}
		if err := s.writeRestoredSite(acc, st); err != nil {
			return err
		}
		if err := s.restoreSiteOptions(created.ID, st.Options); err != nil {
			return err
		}
		if st.WAFRemove != nil {
			_ = s.Store.UpdateSiteWAFRules(created.ID, st.WAFRemove)
		}
		if st.GitRepo != "" {
			token := st.GitToken
			if token == "" {
				token, _ = secret.RandomIdent(16)
			}
			_ = s.Store.UpsertSiteGit(&store.SiteGit{
				SiteID: created.ID, Repo: st.GitRepo, Branch: st.GitBranch, Path: st.GitPath, Command: st.GitCommand, Token: token,
			})
		}
	}
	for _, f := range man.FTP {
		if _, err := s.Store.GetFTPUserByLogin(f.Login); err == nil {
			continue
		}
		if f.Password == "" {
			continue
		}
		if _, err := s.Agent.FTPCreate(rpc.FTPCreateReq{Owner: username, Name: ftpNameFromLogin(username, f.Login), Password: f.Password, Home: ftpHomeRel(username, f.Home)}); err != nil {
			continue
		}
		ftpEnc, _ := s.encryptPassword(f.Password)
		_, _ = s.Store.CreateFTPUser(acc.ID, f.Login, f.Home, ftpEnc)
	}
	return nil
}

func (s *Server) writeRestoredSite(acc *store.Account, st rpc.BackupSite) error {
	php := st.PHPVersion
	if php == "" {
		php = "8.3"
	}
	kind := st.Kind
	if kind == "" {
		kind = "php"
	}
	var opt siteopts.Options
	if st.Options != nil {
		norm, err := siteopts.Normalize(*st.Options)
		if err != nil {
			return err
		}
		opt = norm
	}
	return s.Agent.SiteWrite(rpc.SiteWriteReq{
		Username:   acc.Username,
		Domain:     st.Domain,
		DocRoot:    st.DocRoot,
		PHPVersion: php,
		Enabled:    st.Enabled,
		Aliases:    st.Aliases,
		SSL:        st.SSL,
		SSLKind:    st.SSLKind,
		Rewrite:    st.Rewrite,
		Kind:       kind,
		ProxyPass:  st.ProxyPass,
		AppPort:    st.AppPort,
		AppCmd:     st.AppCmd,
		WAF:        st.WAF,
		WAFRemove:  append([]int(nil), st.WAFRemove...),
		Options:    opt,
		FPM:        s.accountFPMPtr(acc),
	})
}

func (s *Server) restoreSiteOptions(id int64, opt *siteopts.Options) error {
	if opt == nil {
		return nil
	}
	raw, err := siteopts.Marshal(*opt)
	if err != nil {
		return err
	}
	return s.Store.UpdateSiteOptions(id, raw)
}

func ftpHomeRel(owner, home string) string {
	home = strings.ReplaceAll(strings.TrimSpace(home), "\\", "/")
	prefix := "/home/" + owner + "/"
	if strings.HasPrefix(home, prefix) {
		return strings.TrimPrefix(home, prefix)
	}
	if home == "/home/"+owner {
		return ""
	}
	return strings.TrimPrefix(home, "/")
}

func ftpNameFromLogin(owner, login string) string {
	prefix := owner + "_"
	if strings.HasPrefix(login, prefix) {
		return strings.TrimPrefix(login, prefix)
	}
	return login
}

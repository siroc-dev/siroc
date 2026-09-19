package api

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/siroc-dev/siroc/internal/auth"
	"github.com/siroc-dev/siroc/internal/config"
	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/store"
	"github.com/siroc-dev/siroc/internal/validate"
	"github.com/siroc-dev/siroc/internal/version"
)

type Server struct {
	Cfg         config.Config
	Store       *store.Store
	Auth        *auth.Service
	Agent       *rpc.Client
	Static      fs.FS
	installWake chan struct{}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(s.secureHeaders)
	r.Use(s.originCheck)

	r.Get("/api/setup/status", s.setupStatus)
	r.Post("/api/setup", s.setup)
	r.Post("/api/login", s.login)
	r.Post("/api/logout", s.logout)

	r.Group(func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Get("/api/me", s.me)
		r.Post("/api/me/password", s.changePassword)
		r.Post("/api/me/return", s.returnAdmin)
		r.Get("/api/dashboard", s.dashboard)
		r.Get("/api/usage", s.userUsage)
		r.Get("/api/password/suggest", s.suggestPassword)

		r.Get("/api/accounts", s.listAccounts)
		r.Post("/api/accounts", s.createAccount)
		r.Get("/api/accounts/{username}/password", s.showAccountPassword)
		r.Put("/api/accounts/{username}/password", s.setAccountPassword)
		r.Put("/api/accounts/{username}/access", s.setAccountAccess)
		r.Post("/api/accounts/{username}/login-as", s.loginAs)
		r.Post("/api/accounts/{username}/suspend", s.suspendAccount)
		r.Post("/api/accounts/{username}/unsuspend", s.unsuspendAccount)
		r.Put("/api/accounts/{username}/cli", s.setAccountCLI)
		r.Delete("/api/accounts/{username}", s.deleteAccount)

		r.Get("/api/ftp", s.listFTP)
		r.Post("/api/ftp", s.createFTP)
		r.Get("/api/ftp/{id}/password", s.showFTPPassword)
		r.Put("/api/ftp/{id}/password", s.setFTPPassword)
		r.Delete("/api/ftp/{id}", s.deleteFTP)

		r.Get("/api/files", s.listFiles)
		r.Get("/api/files/content", s.readFile)
		r.Put("/api/files/content", s.writeFile)
		r.Post("/api/files/mkdir", s.mkdir)
		r.Post("/api/files/rename", s.renameFile)
		r.Post("/api/files/chmod", s.chmodFile)
		r.Post("/api/files/copy", s.copyFile)
		r.Post("/api/files/extract", s.extractFile)
		r.Delete("/api/files", s.deleteFile)
		r.Post("/api/files/upload", s.uploadFile)
		r.Get("/api/files/download", s.downloadFile)
		r.Get("/api/files/lsp", s.fileLSP)
		r.Get("/api/files/lsp-status", s.fileLSPStatus)
		r.Get("/api/term", s.terminalWS)

		r.Get("/api/software/php-versions", s.phpVersions)
		r.Get("/api/php/extensions", s.phpExtensions)
		r.Get("/api/accounts/{username}/php", s.getAccountPHP)
		r.Put("/api/accounts/{username}/php", s.setAccountPHP)
		r.Group(func(r chi.Router) {
			r.Use(s.requireAdmin)
			r.Get("/api/software", s.listSoftware)
			r.Post("/api/software/install", s.installSoftware)
			r.Get("/api/software/jobs", s.listInstallJobs)
			r.Delete("/api/software/jobs/{id}", s.cancelInstallJob)
			r.Post("/api/software/clean-temp", s.cleanInstallTemp)
			r.Post("/api/software/clean-log", s.cleanInstallLog)
			r.Post("/api/software/service", s.softwareService)
			r.Post("/api/software/cli", s.setCLI)
			r.Get("/api/software/redis", s.redisSettings)
			r.Put("/api/software/redis", s.setRedisSettings)
			r.Get("/api/ssl/letsencrypt", s.leSettings)
			r.Put("/api/ssl/letsencrypt", s.setLESettings)
			r.Post("/api/php/extensions", s.installPHPExt)
			r.Get("/api/security/firewall", s.firewallStatus)
			r.Post("/api/security/firewall/enable", s.firewallEnable)
			r.Post("/api/security/firewall/rules", s.firewallAdd)
			r.Post("/api/security/firewall/delete", s.firewallDelete)
			r.Get("/api/security/waf", s.wafStatus)
			r.Get("/api/security/waf/rules", s.wafRules)
			r.Post("/api/security/waf", s.wafSetMode)
			r.Get("/api/security/av", s.avStatus)
			r.Post("/api/security/av/update", s.avUpdate)
			r.Post("/api/security/av/scan", s.avScan)
			r.Get("/api/security/scanners", s.scannersStatus)
			r.Post("/api/security/scan", s.runScanner)
			r.Get("/api/security/scans/{id}", s.scannerReport)
			r.Get("/api/security/logs", s.scannerLogs)
			r.Get("/api/security/logs/{id}", s.scannerLog)
			r.Get("/api/monitoring", s.systemStats)
			r.Get("/api/apache/status", s.apacheStatus)
			r.Get("/api/nginx/status", s.nginxStatus)
			r.Get("/api/webserver/optimize", s.webOptimize)
			r.Put("/api/webserver/optimize", s.setWebOptimize)
			r.Get("/api/php-fpm/status", s.phpFpmStatus)
			r.Get("/api/mysql/status", s.mysqlStatus)
			r.Get("/api/mariadb/status", s.mariadbStatus)
			r.Get("/api/databases/config", s.dbConfig)
			r.Put("/api/databases/config", s.setDBConfig)
			r.Get("/api/logs", s.logStatus)
			r.Put("/api/logs", s.setLogRetention)
			r.Post("/api/logs/cloudflare", s.refreshCloudflare)
			r.Get("/api/panel/update", s.panelUpdateStatus)
			r.Post("/api/panel/update", s.panelUpdate)
		})

		r.Get("/api/sites", s.listSites)
		r.Post("/api/sites", s.createSite)
		r.Patch("/api/sites/{id}", s.updateSite)
		r.Post("/api/sites/{id}/rename", s.renameSite)
		r.Post("/api/sites/{id}/ssl", s.issueSiteSSL)
		r.Delete("/api/sites/{id}/ssl", s.disableSiteSSL)
		r.Post("/api/sites/{id}/app", s.siteApp)
		r.Post("/api/sites/{id}/runtime", s.siteRuntime)
		r.Post("/api/sites/{id}/wordpress", s.installWordPress)
		r.Get("/api/sites/{id}/stats", s.siteStatsInfo)
		r.Post("/api/sites/{id}/stats", s.siteStatsInfo)
		r.Get("/api/sites/{id}/stats.html", s.siteStatsHTML)
		r.Delete("/api/sites/{id}", s.deleteSite)

		r.Get("/api/databases", s.listDatabases)
		r.Post("/api/databases", s.createDatabase)
		r.Delete("/api/databases/{id}", s.deleteDatabase)
		r.Get("/api/databases/engine", s.dbEngine)
		r.Post("/api/databases/{id}/phpmyadmin", s.pmaAutologin)
		r.Post("/api/files/fetch", s.fetchFile)
		r.Post("/api/files/search", s.searchFiles)
		r.Get("/api/backup/dests", s.listBackupDests)
		r.Post("/api/backup/dests", s.createBackupDest)
		r.Delete("/api/backup/dests/{id}", s.deleteBackupDest)
		r.Get("/api/backup/jobs", s.listBackupJobs)
		r.Post("/api/backup/run", s.runBackup)
		r.Post("/api/backup/restore", s.restoreBackup)
		r.Put("/api/accounts/{username}/quota", s.setAccountQuota)
		r.Get("/api/quota", s.listQuota)
		r.Get("/api/accounts/redis", s.listAccountRedis)
		r.Get("/api/accounts/{username}/redis", s.getAccountRedis)
		r.Put("/api/accounts/{username}/redis", s.setAccountRedis)
		r.Handle("/pma", s.pmaProxy())
		r.Handle("/pma/*", s.pmaProxy())
	})
	r.Group(func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Use(s.requireAdmin)
		r.Get("/api/sysops", s.sysopsStatus)
		r.Post("/api/sysops", s.sysopsApply)
		r.Get("/api/sysops/disk", s.sysopsDisk)
	})

	if s.Static != nil {
		fileServer(r, "/", s.Static)
	}
	return r
}

func (s *Server) ListenAndServe() error {
	s.StartInstallQueue()
	go s.applyStoredSites()
	s.prepareFirstRun()
	h := s.Router()
	if s.Cfg.DisableTLS {
		log.Printf("siroc-panel listening http://%s", s.Cfg.ListenAddr)
		return http.ListenAndServe(s.Cfg.ListenAddr, h)
	}
	if err := ensureTLS(s.Cfg.TLSCert, s.Cfg.TLSKey); err != nil {
		return err
	}
	srv := &http.Server{
		Addr:    s.Cfg.ListenAddr,
		Handler: h,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}
	log.Printf("siroc-panel listening https://%s", s.Cfg.ListenAddr)
	return srv.ListenAndServeTLS(s.Cfg.TLSCert, s.Cfg.TLSKey)
}

type ctxKey int

const userKey ctxKey = 1

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sid := auth.SessionID(r)
		if sid == "" {
			writeErr(w, http.StatusUnauthorized, fmt.Errorf("unauthorized"))
			return
		}
		u, err := s.Store.GetSessionUser(sid)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, fmt.Errorf("unauthorized"))
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}

func currentUser(r *http.Request) *store.PanelUser {
	u, _ := r.Context().Value(userKey).(*store.PanelUser)
	return u
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	ip := r.RemoteAddr
	if !s.Auth.AllowLogin(ip) {
		writeErr(w, http.StatusTooManyRequests, fmt.Errorf("too many login attempts"))
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(w, r, &body) {
		return
	}
	u, err := s.Store.GetPanelUserByName(body.Username)
	if err != nil || !auth.Check(u.PasswordHash, body.Password) {
		s.Auth.Fail(ip)
		writeErr(w, http.StatusUnauthorized, fmt.Errorf("invalid credentials"))
		return
	}
	s.Auth.OK(ip)
	if acc, err := s.Store.GetAccountByName(u.Username); err == nil && acc.Suspended {
		s.Auth.Fail(ip)
		writeErr(w, http.StatusUnauthorized, fmt.Errorf("account is suspended"))
		return
	}
	sid, err := s.Store.CreateSession(u.ID, 0, 7*24*time.Hour)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	s.Auth.SetCookie(w, sid, !s.Cfg.DisableTLS)
	s.Auth.ClearReturnCookie(w, !s.Cfg.DisableTLS)
	writeJSON(w, http.StatusOK, map[string]any{"username": u.Username, "role": u.Role})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if sid := auth.SessionID(r); sid != "" {
		_ = s.Store.DeleteSession(sid)
	}
	s.Auth.ClearCookie(w, !s.Cfg.DisableTLS)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"username":     u.Username,
		"role":         u.Role,
		"impersonator": u.Impersonator,
		"admin":        isAdmin(u),
		"version":      version.Current(),
	})
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Current string `json:"current"`
		Next    string `json:"next"`
	}
	if !decode(w, r, &body) {
		return
	}
	u := currentUser(r)
	if !auth.Check(u.PasswordHash, body.Current) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("current password is wrong"))
		return
	}
	if !auth.ValidPassword(body.Next) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("password must be at least 8 characters"))
		return
	}
	hash, err := auth.Hash(body.Next)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.Store.UpdatePassword(u.ID, hash); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if acc, err := s.Store.GetAccountByName(u.Username); err == nil {
		if err := s.Agent.UserPassword(acc.Username, body.Next); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if enc, err := s.encryptPassword(body.Next); err == nil {
			_ = s.Store.UpdateAccountPassword(acc.Username, enc)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var accounts, sites, dbs int
	var err error
	if isAdmin(u) {
		accounts, sites, dbs, err = s.Store.Counts()
	} else if acc, aerr := s.Store.GetAccountByName(u.Username); aerr == nil {
		accounts = 1
		sites, err = s.Store.CountSites(acc.ID)
		if err == nil {
			dbs, err = s.Store.CountDatabases(acc.ID)
		}
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	health, herr := s.Agent.Health()
	agentOK := herr == nil && health != nil && health.OK
	resp := map[string]any{
		"accounts":  accounts,
		"sites":     sites,
		"databases": dbs,
		"agentOk":   agentOK,
	}
	if isAdmin(u) {
		if st, err := s.Agent.SystemStats(); err == nil && st != nil {
			resp["hostname"] = st.Hostname
			resp["cpuPercent"] = st.CPU.Percent
			resp["cpuCores"] = st.CPU.Cores
			resp["memPercent"] = st.Memory.Percent
			resp["memUsed"] = st.Memory.Used
			resp["memTotal"] = st.Memory.Total
			if len(st.Disks) > 0 {
				root := st.Disks[0]
				for _, d := range st.Disks {
					if d.Mount == "/" {
						root = d
						break
					}
				}
				resp["diskPercent"] = root.Percent
				resp["diskUsed"] = root.Used
				resp["diskTotal"] = root.Total
			}
			resp["load1"] = st.Load.One
			resp["uptimeSec"] = st.UptimeSec
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) userUsage(w http.ResponseWriter, r *http.Request) {
	st, err := s.Agent.SystemStats()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	accounts, err := s.Store.ListAccounts()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	by := map[string]rpc.UserUsage{}
	for _, x := range st.Users {
		by[x.User] = x
	}
	u := currentUser(r)
	admin := isAdmin(u)
	out := make([]rpc.UserUsage, 0, len(accounts))
	for _, a := range accounts {
		if !admin && a.Username != u.Username {
			continue
		}
		if x, ok := by[a.Username]; ok {
			out = append(out, x)
			continue
		}
		out = append(out, rpc.UserUsage{User: a.Username})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) systemStats(w http.ResponseWriter, _ *http.Request) {
	st, err := s.Agent.SystemStats()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) apacheStatus(w http.ResponseWriter, _ *http.Request) {
	st, err := s.Agent.ApacheStatus()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) nginxStatus(w http.ResponseWriter, _ *http.Request) {
	st, err := s.Agent.NginxStatus()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) webOptimize(w http.ResponseWriter, r *http.Request) {
	ram, _ := strconv.Atoi(r.URL.Query().Get("ramGB"))
	st, err := s.Agent.WebOptimize(ram)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) setWebOptimize(w http.ResponseWriter, r *http.Request) {
	var body rpc.WebOptimizeApply
	if !decode(w, r, &body) {
		return
	}
	st, err := s.Agent.SetWebOptimize(body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) phpFpmStatus(w http.ResponseWriter, _ *http.Request) {
	st, err := s.Agent.PHPFPMStatus()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) mysqlStatus(w http.ResponseWriter, _ *http.Request) {
	st, err := s.Agent.DBStatus("mysql")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) mariadbStatus(w http.ResponseWriter, _ *http.Request) {
	st, err := s.Agent.DBStatus("mariadb")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) listAccounts(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListAccounts()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if !isAdmin(currentUser(r)) {
		name := currentUser(r).Username
		filtered := list[:0]
		for _, a := range list {
			if a.Username == name {
				filtered = append(filtered, a)
			}
		}
		list = filtered
		if list == nil {
			list = []store.Account{}
		}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(currentUser(r)) {
		writeErr(w, http.StatusForbidden, fmt.Errorf("admin only"))
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		SSH      *bool  `json:"ssh"`
		FTP      *bool  `json:"ftp"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := validate.LinuxUser(body.Username); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if !auth.ValidPassword(body.Password) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("password must be at least 8 characters"))
		return
	}
	if existing, err := s.Store.GetPanelUserByName(body.Username); err == nil && existing.Role == "admin" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("username is a panel administrator"))
		return
	}
	ssh, ftp := true, true
	if body.SSH != nil {
		ssh = *body.SSH
	}
	if body.FTP != nil {
		ftp = *body.FTP
	}
	u, err := s.Agent.UserCreate(body.Username, body.Password, ssh, ftp)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	enc, err := s.encryptPassword(body.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	hash, err := auth.Hash(body.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.Store.UpsertHostPanelUser(u.Username, hash); err != nil {
		_ = s.Agent.UserDelete(u.Username)
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	acc, err := s.Store.CreateAccount(u.Username, u.UID, u.GID, enc, ssh, ftp)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"account": acc, "password": body.Password})
}

func (s *Server) suspendAccount(w http.ResponseWriter, r *http.Request) {
	s.setSuspended(w, r, true)
}

func (s *Server) unsuspendAccount(w http.ResponseWriter, r *http.Request) {
	s.setSuspended(w, r, false)
}

func (s *Server) setSuspended(w http.ResponseWriter, r *http.Request, suspended bool) {
	if !isAdmin(currentUser(r)) {
		writeErr(w, http.StatusForbidden, fmt.Errorf("admin only"))
		return
	}
	username := chi.URLParam(r, "username")
	if _, err := s.Store.GetAccountByName(username); err != nil {
		writeErr(w, http.StatusNotFound, fmt.Errorf("account not found"))
		return
	}
	if err := s.Agent.UserSuspend(username, suspended); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = s.Store.SetAccountSuspended(username, suspended)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) setAccountCLI(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	if !s.allowAccount(w, r, username) {
		return
	}
	if _, err := s.Store.GetAccountByName(username); err != nil {
		writeErr(w, http.StatusNotFound, fmt.Errorf("account not found"))
		return
	}
	var body rpc.UserCLIReq
	if !decode(w, r, &body) {
		return
	}
	body.Username = username
	if err := s.Agent.UserCLI(body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.Store.UpdateAccountCLI(username, body.PHP, body.Python, body.Node); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	acc, _ := s.Store.GetAccountByName(username)
	writeJSON(w, http.StatusOK, acc)
}

func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(currentUser(r)) {
		writeErr(w, http.StatusForbidden, fmt.Errorf("admin only"))
		return
	}
	username := chi.URLParam(r, "username")
	if _, err := s.Store.GetAccountByName(username); err != nil {
		writeErr(w, http.StatusNotFound, fmt.Errorf("account not found"))
		return
	}
	s.removeAccountFTP(username)
	sites, _ := s.Store.ListSites()
	for _, st := range sites {
		if st.Username == username {
			_ = s.Agent.SiteDelete(username, st.Domain)
			_ = s.Store.DeleteSite(st.ID)
		}
	}
	dbs, _ := s.Store.ListDatabases()
	for _, d := range dbs {
		if d.Username == username {
			_ = s.Agent.DBDrop(rpc.DBDropReq{DBName: d.DBName, DBUser: d.DBUser, Engine: d.Engine})
			_ = s.Store.DeleteDatabase(d.ID)
		}
	}
	if err := s.Agent.UserDelete(username); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = s.Store.DeleteHostPanelUser(username)
	_ = s.Store.DeleteAccount(username)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) accountOrErr(w http.ResponseWriter, r *http.Request, username string) (*store.Account, bool) {
	if username == "" && !isAdmin(currentUser(r)) {
		username = currentUser(r).Username
	}
	acc, err := s.Store.GetAccountByName(username)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("unknown hosting account"))
		return nil, false
	}
	if !s.allowAccount(w, r, acc.Username) {
		return nil, false
	}
	return acc, true
}

func (s *Server) fileUser(w http.ResponseWriter, r *http.Request, username string) (name string, root bool, ok bool) {
	if username == "root" {
		if !isAdmin(currentUser(r)) {
			writeErr(w, http.StatusForbidden, fmt.Errorf("admin only"))
			return "", false, false
		}
		return "root", true, true
	}
	acc, ok := s.accountOrErr(w, r, username)
	if !ok {
		return "", false, false
	}
	return acc.Username, false, true
}

func (s *Server) listFiles(w http.ResponseWriter, r *http.Request) {
	username, root, ok := s.fileUser(w, r, r.URL.Query().Get("user"))
	if !ok {
		return
	}
	out, err := s.Agent.FileList(username, r.URL.Query().Get("path"), root)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) readFile(w http.ResponseWriter, r *http.Request) {
	username, root, ok := s.fileUser(w, r, r.URL.Query().Get("user"))
	if !ok {
		return
	}
	out, err := s.Agent.FileRead(username, r.URL.Query().Get("path"), root)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) writeFile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		User    string `json:"user"`
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if !decode(w, r, &body) {
		return
	}
	username, root, ok := s.fileUser(w, r, body.User)
	if !ok {
		return
	}
	if err := s.Agent.FileWrite(username, body.Path, body.Content, root); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) mkdir(w http.ResponseWriter, r *http.Request) {
	var body struct {
		User string `json:"user"`
		Path string `json:"path"`
	}
	if !decode(w, r, &body) {
		return
	}
	username, root, ok := s.fileUser(w, r, body.User)
	if !ok {
		return
	}
	if err := s.Agent.FileMkdir(username, body.Path, root); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) renameFile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		User string `json:"user"`
		Path string `json:"path"`
		Dest string `json:"dest"`
	}
	if !decode(w, r, &body) {
		return
	}
	username, root, ok := s.fileUser(w, r, body.User)
	if !ok {
		return
	}
	if err := s.Agent.FileRename(username, body.Path, body.Dest, root); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) chmodFile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		User string `json:"user"`
		Path string `json:"path"`
		Mode string `json:"mode"`
	}
	if !decode(w, r, &body) {
		return
	}
	username, root, ok := s.fileUser(w, r, body.User)
	if !ok {
		return
	}
	if err := s.Agent.FileChmod(username, body.Path, body.Mode, root); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) copyFile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		User string `json:"user"`
		Path string `json:"path"`
		Dest string `json:"dest"`
	}
	if !decode(w, r, &body) {
		return
	}
	username, root, ok := s.fileUser(w, r, body.User)
	if !ok {
		return
	}
	if err := s.Agent.FileCopy(username, body.Path, body.Dest, root); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) extractFile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		User string `json:"user"`
		Path string `json:"path"`
	}
	if !decode(w, r, &body) {
		return
	}
	username, root, ok := s.fileUser(w, r, body.User)
	if !ok {
		return
	}
	out, err := s.Agent.FileExtract(username, body.Path, root)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) deleteFile(w http.ResponseWriter, r *http.Request) {
	username, root, ok := s.fileUser(w, r, r.URL.Query().Get("user"))
	if !ok {
		return
	}
	if err := s.Agent.FileDelete(username, r.URL.Query().Get("path"), root); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) uploadFile(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 520<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("upload too large or invalid (max 512MB)"))
		return
	}
	username, root, ok := s.fileUser(w, r, r.FormValue("user"))
	if !ok {
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	defer f.Close()
	dir := strings.TrimSuffix(r.FormValue("path"), "/")
	rel, err := validate.RelUploadPath(r.FormValue("relpath"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	name := filepath.Base(hdr.Filename)
	if name == "." || name == string(filepath.Separator) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid filename"))
		return
	}
	dest := path.Join(dir, name)
	if rel != "" {
		dest = path.Join(dir, rel)
	}
	if dir == "" || dir == "/" {
		if rel != "" {
			dest = "/" + rel
		} else {
			dest = "/" + name
		}
	}
	if err := s.Agent.FileUpload(username, dest, name, f, root); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) downloadFile(w http.ResponseWriter, r *http.Request) {
	username, root, ok := s.fileUser(w, r, r.URL.Query().Get("user"))
	if !ok {
		return
	}
	res, err := s.Agent.FileDownload(username, r.URL.Query().Get("path"), root)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	defer res.Body.Close()
	for _, h := range []string{"Content-Type", "Content-Disposition", "Content-Length"} {
		if v := res.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, res.Body)
}

func (s *Server) listSoftware(w http.ResponseWriter, _ *http.Request) {
	list, err := s.Agent.Packages()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) setCLI(w http.ResponseWriter, r *http.Request) {
	var body rpc.CLISetReq
	if !decode(w, r, &body) {
		return
	}
	switch body.Name {
	case "php", "python", "nodejs":
	default:
		writeErr(w, http.StatusBadRequest, fmt.Errorf("CLI switch is only supported for php, python, and nodejs"))
		return
	}
	if err := s.Agent.SetCLI(body.Name, body.Version); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) softwareService(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name   string `json:"name"`
		Action string `json:"action"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := s.Agent.Service(body.Name, body.Action); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) firewallStatus(w http.ResponseWriter, _ *http.Request) {
	st, err := s.Agent.FirewallStatus()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) firewallEnable(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enable bool `json:"enable"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := s.Agent.FirewallEnable(body.Enable); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) firewallAdd(w http.ResponseWriter, r *http.Request) {
	var body rpc.FirewallRuleReq
	if !decode(w, r, &body) {
		return
	}
	if err := s.Agent.FirewallAdd(body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) firewallDelete(w http.ResponseWriter, r *http.Request) {
	var body rpc.FirewallDeleteReq
	if !decode(w, r, &body) {
		return
	}
	if err := s.Agent.FirewallDelete(body.ID); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) wafStatus(w http.ResponseWriter, _ *http.Request) {
	st, err := s.Agent.WAFStatus()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) wafRules(w http.ResponseWriter, r *http.Request) {
	out, err := s.Agent.WAFRules(r.URL.Query().Get("q"), r.URL.Query().Get("pack"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) wafSetMode(w http.ResponseWriter, r *http.Request) {
	var body rpc.WAFModeReq
	if !decode(w, r, &body) {
		return
	}
	if err := s.Agent.WAFApply(body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) avStatus(w http.ResponseWriter, _ *http.Request) {
	st, err := s.Agent.AVStatus()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) avUpdate(w http.ResponseWriter, _ *http.Request) {
	if err := s.Agent.AVUpdate(); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) avScan(w http.ResponseWriter, r *http.Request) {
	var body rpc.AVScanReq
	if !decode(w, r, &body) {
		return
	}
	out, err := s.Agent.AVScan(body.Path)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) redisSettings(w http.ResponseWriter, _ *http.Request) {
	st, err := s.Agent.Redis()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) setRedisSettings(w http.ResponseWriter, r *http.Request) {
	var body rpc.RedisSettings
	if !decode(w, r, &body) {
		return
	}
	if err := s.Agent.SetRedis(body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	st, _ := s.Agent.Redis()
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) scannersStatus(w http.ResponseWriter, _ *http.Request) {
	st, err := s.Agent.Scanners()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) runScanner(w http.ResponseWriter, r *http.Request) {
	var body rpc.ScanReq
	if !decode(w, r, &body) {
		return
	}
	target, err := validate.ScanTarget(body.Target)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	body.Target = target
	out, err := s.Agent.RunScan(body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) scannerLogs(w http.ResponseWriter, _ *http.Request) {
	list, err := s.Agent.ScanLogs()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) scannerLog(w http.ResponseWriter, r *http.Request) {
	st, err := s.Agent.ScanLog(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) scannerReport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctype, body, err := s.Agent.ScanReport(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	w.Header().Set("Content-Type", ctype)
	_, _ = w.Write(body)
}

func (s *Server) phpVersions(w http.ResponseWriter, _ *http.Request) {
	list, err := s.Agent.PHPVersions()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) listSites(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListSites()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if !isAdmin(currentUser(r)) {
		name := currentUser(r).Username
		filtered := list[:0]
		for _, st := range list {
			if st.Username == name {
				filtered = append(filtered, st)
			}
		}
		list = filtered
		if list == nil {
			list = []store.Site{}
		}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createSite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username   string   `json:"username"`
		Domain     string   `json:"domain"`
		PHPVersion string   `json:"phpVersion"`
		Aliases    []string `json:"aliases"`
		DocRoot    string   `json:"docRoot"`
		Kind       string   `json:"kind"`
		ProxyPass  string   `json:"proxyPass"`
		AppPort    int      `json:"appPort"`
		AppCmd     string   `json:"appCmd"`
		Rewrite    string   `json:"rewrite"`
	}
	if !decode(w, r, &body) {
		return
	}
	acc, ok := s.accountOrErr(w, r, body.Username)
	if !ok {
		return
	}
	body.Domain = strings.ToLower(strings.TrimSpace(body.Domain))
	aliases, err := validate.DomainAliases(body.Domain, body.Aliases)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.claimHostnames(0, append([]string{body.Domain}, aliases...)...); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if body.PHPVersion == "" {
		body.PHPVersion = "8.3"
	}
	kind, err := validate.SiteKind(body.Kind)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	appPort := body.AppPort
	appCmd := strings.TrimSpace(body.AppCmd)
	if validate.IsAppKind(kind) {
		if appCmd == "" {
			appCmd = validate.DefaultAppCmd(kind)
		}
		appCmd, err = validate.AppCommand(appCmd)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if appPort == 0 {
			appPort, err = s.Store.NextAppPort(0)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
		}
		if err := validate.AppPort(appPort); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		taken, err := s.Store.AppPortTaken(appPort, 0)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		if taken {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("app port %d is already used", appPort))
			return
		}
		body.ProxyPass = fmt.Sprintf("http://127.0.0.1:%d/", appPort)
	} else if kind == "proxy" {
		if _, err := validate.ProxyURL(body.ProxyPass); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		appPort = 0
		appCmd = ""
	} else {
		kind = "php"
		body.ProxyPass = ""
		appPort = 0
		appCmd = ""
		if err := s.requirePHPInstalled(body.PHPVersion); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
	}
	doc, err := validate.AccountPath(s.Cfg.HomeRoot, acc.Username, body.DocRoot, body.Domain)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	rewrite := strings.TrimSpace(body.Rewrite)
	if rewrite != "" {
		if _, err = validate.NginxSnippet(rewrite); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
	}
	req := rpc.SiteWriteReq{
		Username:   acc.Username,
		Domain:     body.Domain,
		DocRoot:    doc,
		PHPVersion: body.PHPVersion,
		Enabled:    true,
		Aliases:    aliases,
		SSL:        true,
		SSLKind:    "local",
		Rewrite:    rewrite,
		FPM:        s.accountFPMPtr(acc),
		Kind:       kind,
		ProxyPass:  body.ProxyPass,
		AppPort:    appPort,
		AppCmd:     appCmd,
	}
	if err := s.Agent.SiteWrite(req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	st, err := s.Store.CreateSite(acc.ID, body.Domain, doc, body.PHPVersion, aliases, kind, body.ProxyPass, rewrite, appPort, appCmd)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) updateSite(w http.ResponseWriter, r *http.Request) {
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
		PHPVersion string   `json:"phpVersion"`
		Enabled    *bool    `json:"enabled"`
		Aliases    []string `json:"aliases"`
		Rewrite    *string  `json:"rewrite"`
		SSL        *bool    `json:"ssl"`
		SSLKind    *string  `json:"sslKind"`
		DocRoot    *string  `json:"docRoot"`
		Kind       *string  `json:"kind"`
		ProxyPass  *string  `json:"proxyPass"`
		AppPort    *int     `json:"appPort"`
		AppCmd     *string  `json:"appCmd"`
	}
	if !decode(w, r, &body) {
		return
	}
	php := st.PHPVersion
	nextKind := st.Kind
	if body.Kind != nil {
		k, kerr := validate.SiteKind(*body.Kind)
		if kerr != nil {
			writeErr(w, http.StatusBadRequest, kerr)
			return
		}
		nextKind = k
	}
	if body.PHPVersion != "" && !validate.IsAppKind(nextKind) && nextKind != "proxy" {
		if err := s.requirePHPInstalled(body.PHPVersion); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		php = body.PHPVersion
	}
	enabled := st.Enabled
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	aliases := st.Aliases
	if body.Aliases != nil {
		aliases, err = validate.DomainAliases(st.Domain, body.Aliases)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if err := s.claimHostnames(st.ID, aliases...); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
	}
	rewrite := st.Rewrite
	if body.Rewrite != nil {
		if _, err = validate.NginxSnippet(*body.Rewrite); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		rewrite = strings.TrimSpace(*body.Rewrite)
	}
	doc := st.DocRoot
	if body.DocRoot != nil {
		doc, err = validate.AccountPath(s.Cfg.HomeRoot, st.Username, *body.DocRoot, st.Domain)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
	}
	st.DocRoot = doc
	ssl := st.SSL
	kind := st.SSLKind
	expiry := st.SSLExpiry
	if body.SSLKind != nil {
		switch strings.TrimSpace(*body.SSLKind) {
		case "local":
			ssl = true
			kind = "local"
		case "letsencrypt":
			ssl = true
			kind = "letsencrypt"
		case "":
			ssl = false
			kind = ""
		default:
			writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid ssl kind"))
			return
		}
	}
	if body.SSL != nil {
		ssl = *body.SSL
		if ssl && kind == "" {
			kind = "local"
		}
		if !ssl {
			kind = ""
		}
	}
	st.Kind = nextKind
	if body.AppCmd != nil {
		st.AppCmd = strings.TrimSpace(*body.AppCmd)
	}
	if body.AppPort != nil {
		st.AppPort = *body.AppPort
	}
	if body.ProxyPass != nil {
		st.ProxyPass = strings.TrimSpace(*body.ProxyPass)
	}
	if validate.IsAppKind(st.Kind) {
		if st.AppCmd == "" {
			st.AppCmd = validate.DefaultAppCmd(st.Kind)
		}
		cmd, err := validate.AppCommand(st.AppCmd)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		st.AppCmd = cmd
		if st.AppPort == 0 {
			st.AppPort, err = s.Store.NextAppPort(st.ID)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
		}
		if err := validate.AppPort(st.AppPort); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		taken, err := s.Store.AppPortTaken(st.AppPort, st.ID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		if taken {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("app port %d is already used", st.AppPort))
			return
		}
		st.ProxyPass = fmt.Sprintf("http://127.0.0.1:%d/", st.AppPort)
	} else if st.Kind == "proxy" {
		st.AppCmd = ""
		st.AppPort = 0
		if st.ProxyPass == "" {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("proxy URL required"))
			return
		}
		if _, err := validate.ProxyURL(st.ProxyPass); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
	} else {
		st.Kind = "php"
		st.ProxyPass = ""
		st.AppCmd = ""
		st.AppPort = 0
	}
	req := s.siteWriteReq(*st, php, enabled, aliases, ssl, kind, rewrite)
	if err := s.Agent.SiteWrite(req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if ssl && kind == "letsencrypt" && body.Aliases != nil {
		out, err := s.Agent.SiteSSL(s.siteSSLReq(st.Username, st.Domain, aliases, ""))
		if err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("aliases saved locally but Let's Encrypt expand failed: %w", err))
			_ = s.Store.UpdateSite(st.ID, php, doc, enabled, aliases, ssl, expiry, kind, rewrite)
			return
		}
		expiry = out.Expiry
		kind = "letsencrypt"
		req.SSL = true
		req.SSLKind = "letsencrypt"
		_ = s.Agent.SiteWrite(req)
	}
	_ = s.Store.UpdateSite(st.ID, php, doc, enabled, aliases, ssl, expiry, kind, rewrite)
	_ = s.Store.UpdateSiteApp(st.ID, st.Kind, st.ProxyPass, st.AppPort, st.AppCmd)
	st, _ = s.Store.GetSite(id)
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) issueSiteSSL(w http.ResponseWriter, r *http.Request) {
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
		Email string `json:"email"`
	}
	if r.ContentLength != 0 && !decode(w, r, &body) {
		return
	}
	if err := validate.Email(body.Email); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	req := s.siteWriteReq(*st, st.PHPVersion, st.Enabled, st.Aliases, false, "", st.Rewrite)
	if err := s.Agent.SiteWrite(req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	out, err := s.Agent.SiteSSL(s.siteSSLReq(st.Username, st.Domain, st.Aliases, body.Email))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	req.SSL = true
	req.SSLKind = "letsencrypt"
	if err := s.Agent.SiteWrite(req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = s.Store.UpdateSite(st.ID, st.PHPVersion, st.DocRoot, st.Enabled, st.Aliases, true, out.Expiry, "letsencrypt", st.Rewrite)
	st, _ = s.Store.GetSite(id)
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) disableSiteSSL(w http.ResponseWriter, r *http.Request) {
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
	req := s.siteWriteReq(*st, st.PHPVersion, st.Enabled, st.Aliases, false, "", st.Rewrite)
	if err := s.Agent.SiteWrite(req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = s.Store.UpdateSite(st.ID, st.PHPVersion, st.DocRoot, st.Enabled, st.Aliases, false, st.SSLExpiry, "", st.Rewrite)
	st, _ = s.Store.GetSite(id)
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) siteWriteReq(st store.Site, php string, enabled bool, aliases []string, ssl bool, kind, rewrite string) rpc.SiteWriteReq {
	if ssl && kind == "" {
		kind = "local"
	}
	if !ssl {
		kind = ""
	}
	return rpc.SiteWriteReq{
		Username:   st.Username,
		Domain:     st.Domain,
		DocRoot:    st.DocRoot,
		PHPVersion: php,
		Enabled:    enabled,
		Aliases:    aliases,
		SSL:        ssl,
		SSLKind:    kind,
		Rewrite:    rewrite,
		FPM:        s.accountFPMPtrByName(st.Username),
		Kind:       st.Kind,
		ProxyPass:  st.ProxyPass,
		AppPort:    st.AppPort,
		AppCmd:     st.AppCmd,
	}
}

func (s *Server) siteApp(w http.ResponseWriter, r *http.Request) {
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
	var body rpc.SiteAppReq
	if r.ContentLength != 0 && !decode(w, r, &body) {
		return
	}
	body.Username = st.Username
	body.Domain = st.Domain
	body.DocRoot = st.DocRoot
	body.PHPVersion = st.PHPVersion
	out, err := s.Agent.SiteApp(body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) siteRuntime(w http.ResponseWriter, r *http.Request) {
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
	if !validate.IsAppKind(st.Kind) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("this site is not a Node.js, Python, Go, Rust, or Docker app"))
		return
	}
	var body struct {
		Action string `json:"action"`
	}
	if r.ContentLength != 0 && !decode(w, r, &body) {
		return
	}
	out, err := s.Agent.SiteRuntime(rpc.SiteRuntimeReq{
		Username: st.Username,
		Domain:   st.Domain,
		DocRoot:  st.DocRoot,
		Kind:     st.Kind,
		AppPort:  st.AppPort,
		AppCmd:   st.AppCmd,
		Action:   body.Action,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) applyStoredSites() {
	for i := 0; i < 30; i++ {
		if _, err := s.Agent.Health(); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	migrated, _ := s.Store.Setting("sites_local_ssl")
	sites, err := s.Store.ListSites()
	if err != nil {
		log.Printf("apply sites: %v", err)
		return
	}
	for _, st := range sites {
		kind := st.SSLKind
		ssl := st.SSL
		if migrated != "1" && kind == "" {
			ssl = true
			kind = "local"
			_ = s.Store.UpdateSite(st.ID, st.PHPVersion, st.DocRoot, st.Enabled, st.Aliases, true, st.SSLExpiry, "local", st.Rewrite)
		}
		if migrated == "1" || !ssl {
			continue
		}
		req := s.siteWriteReq(st, st.PHPVersion, st.Enabled, st.Aliases, true, kind, st.Rewrite)
		if err := s.Agent.SiteWrite(req); err != nil {
			log.Printf("apply ssl %s: %v", st.Domain, err)
		}
	}
	if migrated != "1" {
		_ = s.Store.SetSetting("sites_local_ssl", "1")
	}
}

func (s *Server) claimHostnames(exceptID int64, names ...string) error {
	for _, n := range names {
		taken, err := s.Store.HostnameTaken(n, exceptID)
		if err != nil {
			return err
		}
		if taken {
			return fmt.Errorf("%s is already used by another website", n)
		}
	}
	return nil
}

func (s *Server) deleteSite(w http.ResponseWriter, r *http.Request) {
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
	if err := s.Agent.SiteDelete(st.Username, st.Domain); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = s.Store.DeleteSite(id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func replaceSiteDomainPath(home, oldDomain, newDomain, doc string) string {
	oldBase := filepath.Join(home, "domains", oldDomain)
	newBase := filepath.Join(home, "domains", newDomain)
	clean := filepath.Clean(doc)
	if clean == oldBase || strings.HasPrefix(clean, oldBase+string(os.PathSeparator)) {
		return newBase + strings.TrimPrefix(clean, oldBase)
	}
	return doc
}

func (s *Server) renameSite(w http.ResponseWriter, r *http.Request) {
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
		Domain string `json:"domain"`
	}
	if !decode(w, r, &body) {
		return
	}
	newDomain := strings.ToLower(strings.TrimSpace(body.Domain))
	if err := validate.Domain(newDomain); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if newDomain == st.Domain {
		writeJSON(w, http.StatusOK, st)
		return
	}
	if err := s.claimHostnames(st.ID, newDomain); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	aliases := make([]string, 0, len(st.Aliases))
	for _, a := range st.Aliases {
		if a != newDomain {
			aliases = append(aliases, a)
		}
	}
	home, _, err := validate.HomeJail(s.Cfg.HomeRoot, st.Username)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	doc := replaceSiteDomainPath(home, st.Domain, newDomain, st.DocRoot)
	kind := st.SSLKind
	expiry := st.SSLExpiry
	if st.SSL {
		kind = "local"
		expiry = ""
	}
	next := *st
	next.Domain = newDomain
	next.DocRoot = doc
	next.Aliases = aliases
	req := s.siteWriteReq(next, st.PHPVersion, st.Enabled, aliases, st.SSL, kind, st.Rewrite)
	if err := s.Agent.SiteRename(rpc.SiteRenameReq{SiteWriteReq: req, OldDomain: st.Domain}); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.Store.RenameSite(st.ID, newDomain, st.PHPVersion, doc, st.Enabled, aliases, st.SSL, expiry, kind, st.Rewrite); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = s.Store.UpdateSiteApp(st.ID, st.Kind, st.ProxyPass, st.AppPort, st.AppCmd)
	st, _ = s.Store.GetSite(id)
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) listDatabases(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListDatabases()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if !isAdmin(currentUser(r)) {
		name := currentUser(r).Username
		filtered := list[:0]
		for _, d := range list {
			if d.Username == name {
				filtered = append(filtered, d)
			}
		}
		list = filtered
		if list == nil {
			list = []store.Database{}
		}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createDatabase(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		DBName   string `json:"dbName"`
		DBUser   string `json:"dbUser"`
		Password string `json:"password"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := validate.DBIdent(body.DBName); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := validate.DBIdent(body.DBUser); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if !auth.ValidPassword(body.Password) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("password must be at least 8 characters"))
		return
	}
	acc, ok := s.accountOrErr(w, r, body.Username)
	if !ok {
		return
	}
	engine, err := s.Agent.DBEngine()
	if err != nil || engine == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("MySQL/MariaDB is not installed"))
		return
	}
	name := acc.Username + "_" + body.DBName
	user := acc.Username + "_" + body.DBUser
	if err := s.Agent.DBCreate(rpc.DBCreateReq{DBName: name, DBUser: user, Password: body.Password, Engine: engine}); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	enc, err := s.encryptPassword(body.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	db, err := s.Store.CreateDatabase(acc.ID, name, user, engine, enc)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"database": db, "password": body.Password})
}

func (s *Server) deleteDatabase(w http.ResponseWriter, r *http.Request) {
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
	if err := s.Agent.DBDrop(rpc.DBDropReq{DBName: db.DBName, DBUser: db.DBUser, Engine: db.Engine}); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = s.Store.DeleteDatabase(id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) dbEngine(w http.ResponseWriter, _ *http.Request) {
	engine, err := s.Agent.DBEngine()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"engine": engine})
}

func (s *Server) dbConfig(w http.ResponseWriter, r *http.Request) {
	ram, _ := strconv.Atoi(r.URL.Query().Get("ramGB"))
	st, err := s.Agent.DBConfig(ram)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) setDBConfig(w http.ResponseWriter, r *http.Request) {
	var body rpc.DBConfigApply
	if !decode(w, r, &body) {
		return
	}
	st, err := s.Agent.SetDBConfig(body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) originCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}
		u := origin
		host := r.Host
		if strings.Contains(u, "://") {
			u = u[strings.Index(u, "://")+3:]
		}
		if u != host {
			writeErr(w, http.StatusForbidden, fmt.Errorf("invalid origin"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid json"))
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func fileServer(r chi.Router, _ string, root fs.FS) {
	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		p := strings.TrimPrefix(req.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		f, err := root.Open(p)
		if err == nil {
			defer f.Close()
			if stat, err := f.Stat(); err == nil && !stat.IsDir() {
				http.ServeFileFS(w, req, root, p)
				return
			}
		}
		http.ServeFileFS(w, req, root, "index.html")
	})
}

func ensureTLS(certFile, keyFile string) error {
	if _, err := os.Stat(certFile); err == nil {
		if _, err := os.Stat(keyFile); err == nil {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(certFile), 0750); err != nil {
		return err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "siroc"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour * 3),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	if err := os.WriteFile(certFile, certPEM, 0640); err != nil {
		return err
	}
	return os.WriteFile(keyFile, keyPEM, 0600)
}

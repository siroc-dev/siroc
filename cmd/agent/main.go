//go:build linux

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/go-chi/chi/v5"

	"github.com/siroc-dev/siroc/internal/backup"
	"github.com/siroc-dev/siroc/internal/config"
	"github.com/siroc-dev/siroc/internal/dbmgmt"
	"github.com/siroc-dev/siroc/internal/files"
	"github.com/siroc-dev/siroc/internal/hosting"
	"github.com/siroc-dev/siroc/internal/monitoring"
	"github.com/siroc-dev/siroc/internal/pma"
	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/security"
	"github.com/siroc-dev/siroc/internal/software"
	"github.com/siroc-dev/siroc/internal/sysops"
	"github.com/siroc-dev/siroc/internal/tty"
	"github.com/siroc-dev/siroc/internal/update"
	"github.com/siroc-dev/siroc/internal/users"
	"github.com/siroc-dev/siroc/internal/version"
	"github.com/siroc-dev/siroc/internal/weblog"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "file-helper":
			files.HelperMain()
			return
		case "cloudflare-ips":
			if os.Geteuid() != 0 {
				log.Fatal("siroc-agent must run as root")
			}
			if err := weblog.UpdateCloudflare(true); err != nil {
				log.Fatal(err)
			}
			return
		case "weblog":
			if os.Geteuid() != 0 {
				log.Fatal("siroc-agent must run as root")
			}
			if err := weblog.Daily(); err != nil {
				log.Fatal(err)
			}
			return
		}
	}
	if os.Geteuid() != 0 {
		log.Fatal("siroc-agent must run as root")
	}
	log.Printf("siroc-agent %s", version.Current())

	cfg := config.Load()
	sockDir := filepath.Dir(cfg.SocketPath)
	if err := os.MkdirAll(sockDir, 0750); err != nil {
		log.Fatal(err)
	}
	if gid := lookupGID("siroc"); gid > 0 {
		_ = os.Chown(sockDir, 0, gid)
	}
	_ = os.Chmod(sockDir, 0750)
	_ = os.Remove(cfg.SocketPath)

	software.PrepareRuntime()

	usersMgr := &users.Manager{HomeRoot: cfg.HomeRoot}
	filesMgr := &files.Manager{HomeRoot: cfg.HomeRoot}
	softMgr := &software.Manager{}
	hostMgr := &hosting.Manager{HomeRoot: cfg.HomeRoot, NginxSites: cfg.NginxSites, ApacheSites: cfg.ApacheSites}
	dbMgr := &dbmgmt.Manager{}
	secMgr := &security.Manager{HomeRoot: cfg.HomeRoot}
	mon := &monitoring.Collector{}
	weblog.Ensure()
	hostMgr.FixAllWebPerms()
	hostMgr.LockOpenBasedir()

	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, rpc.HealthResp{OK: true, Root: true, Socket: cfg.SocketPath})
	})
	r.Get("/update/status", func(w http.ResponseWriter, _ *http.Request) {
		ch := os.Getenv("SIROC_UPDATE_URL")
		writeJSON(w, http.StatusOK, update.Status(ch))
	})
	r.Post("/update", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.PanelUpdateReq
		if !decode(w, r, &req) {
			return
		}
		channel := strings.TrimSpace(req.Channel)
		if channel == "" {
			channel = os.Getenv("SIROC_UPDATE_URL")
		}
		var (
			out *rpc.PanelUpdateStatus
			err error
		)
		switch strings.ToLower(strings.TrimSpace(req.Action)) {
		case "", "status":
			out = update.Status(channel)
		case "check":
			out, err = update.Check(channel)
		case "apply":
			out, err = update.Apply(channel, req.URL, req.Path)
		default:
			writeErr(w, http.StatusBadRequest, fmt.Errorf("unknown update action"))
			return
		}
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if out.Version == "" {
			out.Version = version.Current()
		}
		writeJSON(w, http.StatusOK, out)
	})

	r.Get("/users", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, []rpc.UserResp{})
	})
	r.Post("/users", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.UserCreateReq
		if !decode(w, r, &req) {
			return
		}
		u, err := usersMgr.Create(req.Username, req.Password)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = softMgr.EnsureAccess()
		if err := usersMgr.SetSSH(req.Username, req.SSH); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if err := software.SetFTPAllowed(req.Username, req.FTP); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, u)
	})
	r.Post("/users/password", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.UserPasswordReq
		if !decode(w, r, &req) {
			return
		}
		if err := usersMgr.SetPassword(req.Username, req.Password); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})
	r.Post("/users/access", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.UserAccessReq
		if !decode(w, r, &req) {
			return
		}
		_ = softMgr.EnsureAccess()
		if err := usersMgr.SetSSH(req.Username, req.SSH); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if err := software.SetFTPAllowed(req.Username, req.FTP); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})
	r.Post("/ftp", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.FTPCreateReq
		if !decode(w, r, &req) {
			return
		}
		_ = softMgr.EnsureAccess()
		u, err := usersMgr.CreateFTP(req.Owner, req.Name, req.Password, req.Home)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, u)
	})
	r.Post("/ftp/password", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.FTPActionReq
		if !decode(w, r, &req) {
			return
		}
		if err := usersMgr.SetPassword(req.Login, req.Password); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})
	r.Post("/ftp/delete", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.FTPActionReq
		if !decode(w, r, &req) {
			return
		}
		if err := usersMgr.DeleteKeepHome(req.Login); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})
	r.Post("/users/suspend", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.UserActionReq
		if !decode(w, r, &req) {
			return
		}
		if err := usersMgr.Suspend(req.Username, true); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})
	r.Post("/users/unsuspend", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.UserActionReq
		if !decode(w, r, &req) {
			return
		}
		if err := usersMgr.Suspend(req.Username, false); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})
	r.Post("/users/cli", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.UserCLIReq
		if !decode(w, r, &req) {
			return
		}
		if err := softMgr.ApplyUserCLI(req); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})
	r.Post("/users/delete", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.UserActionReq
		if !decode(w, r, &req) {
			return
		}
		softMgr.DropUserRedis(req.Username)
		if err := usersMgr.Delete(req.Username); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})

	r.Post("/files/list", fileHandler(filesMgr, "list"))
	r.Post("/files/read", fileHandler(filesMgr, "read"))
	r.Post("/files/write", fileHandler(filesMgr, "write"))
	r.Post("/files/mkdir", fileHandler(filesMgr, "mkdir"))
	r.Post("/files/delete", fileHandler(filesMgr, "delete"))
	r.Post("/files/rename", fileHandler(filesMgr, "rename"))
	r.Post("/files/chmod", fileHandler(filesMgr, "chmod"))
	r.Post("/files/copy", fileHandler(filesMgr, "copy"))
	r.Post("/files/extract", fileHandler(filesMgr, "extract"))
	r.Get("/files/download", func(w http.ResponseWriter, r *http.Request) {
		username := r.URL.Query().Get("username")
		path := r.URL.Query().Get("path")
		root := r.URL.Query().Get("root") == "1"
		if err := filesMgr.ServeDownload(w, username, path, root); err != nil {
			writeErr(w, http.StatusBadRequest, err)
		}
	})
	r.Get("/files/lsp", func(w http.ResponseWriter, r *http.Request) {
		files.ServeLSP(w, r, filesMgr, cfg.InstallRoot)
	})
	r.Get("/files/lsp-status", func(w http.ResponseWriter, r *http.Request) {
		lang := r.URL.Query().Get("lang")
		writeJSON(w, http.StatusOK, map[string]any{"ok": files.HasLSP(lang, cfg.InstallRoot), "lang": lang})
	})
	r.Get("/term", func(w http.ResponseWriter, r *http.Request) {
		tty.Serve(w, r)
	})
	r.Post("/files/upload", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 520<<20)
		_ = r.ParseMultipartForm(32 << 20)
		username := r.FormValue("username")
		path := r.FormValue("path")
		f, _, err := r.FormFile("file")
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		defer f.Close()
		if err := filesMgr.Upload(username, path, f, r.FormValue("root") == "1"); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})

	r.Get("/software", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, softMgr.List())
	})
	r.Get("/software/php-versions", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, softMgr.PHPInstalled())
	})
	r.Post("/software/install", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.PkgInstallReq
		if !decode(w, r, &req) {
			return
		}
		if err := softMgr.Install(req.Name, req.Version); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true, Message: "installed"})
	})
	r.Post("/software/cli", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.CLISetReq
		if !decode(w, r, &req) {
			return
		}
		if err := softMgr.SetCLI(req.Name, req.Version); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})
	r.Post("/software/service", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.ServiceReq
		if !decode(w, r, &req) {
			return
		}
		if err := softMgr.Service(req.Name, req.Action); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})
	r.Get("/software/redis", func(w http.ResponseWriter, _ *http.Request) {
		st, err := softMgr.Redis()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, st)
	})
	r.Post("/software/redis", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.RedisSettings
		if !decode(w, r, &req) {
			return
		}
		if err := softMgr.SetRedis(req); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})
	r.Get("/users/redis", func(w http.ResponseWriter, _ *http.Request) {
		st, err := softMgr.ListUserRedis()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, st)
	})
	r.Post("/users/redis", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.UserRedisReq
		if !decode(w, r, &req) {
			return
		}
		st, err := softMgr.UserRedis(req.Username)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, st)
	})
	r.Post("/users/redis/set", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.UserRedisReq
		if !decode(w, r, &req) {
			return
		}
		st, err := softMgr.SetUserRedis(req)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, st)
	})
	r.Post("/cleanup/temp", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, softMgr.CleanTemp())
	})
	r.Post("/cleanup/logs", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, softMgr.CleanLogs())
	})

	r.Post("/sites/write", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.SiteWriteReq
		if !decode(w, r, &req) {
			return
		}
		if err := hostMgr.Write(req); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})
	r.Get("/php/extensions", func(w http.ResponseWriter, r *http.Request) {
		ver := r.URL.Query().Get("version")
		writeJSON(w, http.StatusOK, softMgr.ListPHPExt(ver))
	})
	r.Post("/php/apply", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.PHPUserApplyReq
		if !decode(w, r, &req) {
			return
		}
		if err := hostMgr.ApplyUserPHP(req); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})
	r.Post("/sites/ssl", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.SiteSSLReq
		if !decode(w, r, &req) {
			return
		}
		out, err := hostMgr.IssueSSL(req)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/sites/app", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.SiteAppReq
		if !decode(w, r, &req) {
			return
		}
		out, err := hostMgr.SiteApp(req)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/sites/runtime", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.SiteRuntimeReq
		if !decode(w, r, &req) {
			return
		}
		out, err := hostMgr.SiteRuntime(req)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/sites/delete", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.SiteDeleteReq
		if !decode(w, r, &req) {
			return
		}
		if err := hostMgr.Delete(req.Username, req.Domain); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})
	r.Post("/sites/rename", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.SiteRenameReq
		if !decode(w, r, &req) {
			return
		}
		if err := hostMgr.Rename(req); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})

	r.Get("/db/engine", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true, Message: dbMgr.Engine()})
	})
	r.Get("/db/config", func(w http.ResponseWriter, r *http.Request) {
		ram, _ := strconv.Atoi(r.URL.Query().Get("ramGB"))
		writeJSON(w, http.StatusOK, dbMgr.Config(ram))
	})
	r.Post("/db/config", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.DBConfigApply
		if !decode(w, r, &req) {
			return
		}
		if err := dbMgr.ApplyConfig(req); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, dbMgr.Config(req.RAMGB))
	})
	r.Post("/db/create", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.DBCreateReq
		if !decode(w, r, &req) {
			return
		}
		if err := dbMgr.Create(req); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})
	r.Get("/security/firewall", func(w http.ResponseWriter, _ *http.Request) {
		st, err := secMgr.FirewallStatus()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, st)
	})
	r.Post("/security/firewall/enable", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Enable bool `json:"enable"`
		}
		if !decode(w, r, &req) {
			return
		}
		if err := secMgr.FirewallEnable(req.Enable); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})
	r.Post("/security/firewall/rules", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.FirewallRuleReq
		if !decode(w, r, &req) {
			return
		}
		if err := secMgr.FirewallAdd(req); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})
	r.Post("/security/firewall/delete", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.FirewallDeleteReq
		if !decode(w, r, &req) {
			return
		}
		if err := secMgr.FirewallDelete(req.ID); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})
	r.Get("/security/waf", func(w http.ResponseWriter, _ *http.Request) {
		st, err := secMgr.WAFStatus()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, st)
	})
	r.Get("/security/waf/rules", func(w http.ResponseWriter, r *http.Request) {
		out, err := secMgr.WAFRules(r.URL.Query().Get("q"), r.URL.Query().Get("pack"), 200)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/security/waf", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.WAFModeReq
		if !decode(w, r, &req) {
			return
		}
		if err := secMgr.WAFApply(req); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})
	r.Get("/security/av", func(w http.ResponseWriter, _ *http.Request) {
		st, err := secMgr.AVStatus()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, st)
	})
	r.Post("/security/av/update", func(w http.ResponseWriter, _ *http.Request) {
		if err := secMgr.AVUpdate(); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})
	r.Post("/security/av/scan", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.AVScanReq
		if !decode(w, r, &req) {
			return
		}
		out, err := secMgr.AVScan(req.Path)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Get("/security/scanners", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, secMgr.Scanners())
	})
	r.Post("/security/scan", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.ScanReq
		if !decode(w, r, &req) {
			return
		}
		out, err := secMgr.RunScan(req)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Get("/security/scan/logs", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, secMgr.ListScanLogs())
	})
	r.Get("/security/scan/log", func(w http.ResponseWriter, r *http.Request) {
		st, err := secMgr.GetScanLog(r.URL.Query().Get("id"))
		if err != nil {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, http.StatusOK, st)
	})
	r.Get("/security/scan/report", func(w http.ResponseWriter, r *http.Request) {
		ctype, body, err := secMgr.ScanReport(r.URL.Query().Get("id"))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		w.Header().Set("Content-Type", ctype)
		_, _ = w.Write(body)
	})

	r.Get("/system/stats", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, mon.Stats())
	})
	r.Get("/apache/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, software.ApacheStatus())
	})
	r.Get("/nginx/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, software.NginxStatus())
	})
	r.Get("/webserver/optimize", func(w http.ResponseWriter, r *http.Request) {
		ram, _ := strconv.Atoi(r.URL.Query().Get("ramGB"))
		writeJSON(w, http.StatusOK, software.WebOptimize(ram))
	})
	r.Post("/webserver/optimize", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.WebOptimizeApply
		if !decode(w, r, &req) {
			return
		}
		if err := software.ApplyWebOptimize(req); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, software.WebOptimize(req.RAMGB))
	})
	r.Get("/php-fpm/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, software.PHPFPMStatus())
	})
	r.Get("/mysql/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, software.DBStatus("mysql"))
	})
	r.Get("/mariadb/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, software.DBStatus("mariadb"))
	})
	r.Get("/logs/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, weblog.Status())
	})
	r.Post("/logs/retention", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.LogRetentionReq
		if !decode(w, r, &req) {
			return
		}
		if err := weblog.SetRetention(req.Days); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, weblog.Status())
	})
	r.Post("/logs/cloudflare", func(w http.ResponseWriter, _ *http.Request) {
		if err := weblog.UpdateCloudflare(true); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, weblog.Status())
	})
	r.Post("/logs/goaccess", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.GoAccessReq
		if !decode(w, r, &req) {
			return
		}
		info := goAccessInfo(req.Domain, req.Refresh)
		if !info.OK {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("%s", info.Message))
			return
		}
		writeJSON(w, http.StatusOK, info)
	})
	r.Get("/logs/goaccess.html", func(w http.ResponseWriter, r *http.Request) {
		domain := r.URL.Query().Get("domain")
		info := goAccessInfo(domain, r.URL.Query().Get("refresh") == "1")
		if !info.OK {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("%s", info.Message))
			return
		}
		b, err := weblog.ReadReport(domain)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(b)
	})

	r.Post("/db/drop", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.DBDropReq
		if !decode(w, r, &req) {
			return
		}
		if err := dbMgr.Drop(req); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})
	r.Post("/db/password", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.DBPasswordReq
		if !decode(w, r, &req) {
			return
		}
		if err := dbMgr.SetPassword(req.DBUser, req.Password); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})
	r.Post("/files/fetch", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.FileReq
		if !decode(w, r, &req) {
			return
		}
		out, err := filesMgr.Fetch(req.Username, req.Path, req.URL, req.Dest, req.Root)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/files/search", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.FileReq
		if !decode(w, r, &req) {
			return
		}
		out, err := filesMgr.Search(req.Username, req.Path, req.Query, req.Root)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/sysops", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.SysopsReq
		if r.ContentLength != 0 && !decode(w, r, &req) {
			return
		}
		out, err := sysops.Apply(req)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Get("/sysops/disk", func(w http.ResponseWriter, _ *http.Request) {
		out, err := sysops.Disk()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/users/quota", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.QuotaReq
		if !decode(w, r, &req) {
			return
		}
		out, err := sysops.GetQuota(req.Username)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/users/quota/set", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.QuotaReq
		if !decode(w, r, &req) {
			return
		}
		out, err := sysops.SetQuota(req.Username, req.LimitMB)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/backup/run", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.BackupReq
		if !decode(w, r, &req) {
			return
		}
		out, err := backup.Run(req)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/pma/signon", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.PMASignonReq
		if !decode(w, r, &req) {
			return
		}
		out, err := pma.Signon(req)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})

	ln, err := net.Listen("unix", cfg.SocketPath)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.Chmod(cfg.SocketPath, 0660); err != nil {
		log.Fatal(err)
	}
	if err := os.Chown(cfg.SocketPath, 0, lookupGID("siroc")); err != nil {
		log.Printf("warning: chown socket: %v", err)
	}
	if gid := lookupGID("siroc"); gid > 0 {
		_ = os.Chown(filepath.Dir(cfg.SocketPath), 0, gid)
		_ = os.Chmod(filepath.Dir(cfg.SocketPath), 0750)
	}

	srv := &http.Server{Handler: r}
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		_ = srv.Close()
		_ = os.Remove(cfg.SocketPath)
	}()

	log.Printf("siroc-agent listening on %s", cfg.SocketPath)
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func fileHandler(m *files.Manager, op string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req rpc.FileReq
		if !decode(w, r, &req) {
			return
		}
		var (
			out any
			err error
		)
		switch op {
		case "list":
			out, err = m.List(req.Username, req.Path, req.Root)
		case "read":
			out, err = m.Read(req.Username, req.Path, req.Root)
		case "write":
			err = m.Write(req.Username, req.Path, req.Content, req.Root)
			out = rpc.OKResp{OK: true}
		case "mkdir":
			err = m.Mkdir(req.Username, req.Path, req.Root)
			out = rpc.OKResp{OK: true}
		case "delete":
			err = m.Delete(req.Username, req.Path, req.Root)
			out = rpc.OKResp{OK: true}
		case "rename":
			err = m.Rename(req.Username, req.Path, req.Dest, req.Root)
			out = rpc.OKResp{OK: true}
		case "chmod":
			err = m.Chmod(req.Username, req.Path, req.Mode, req.Root)
			out = rpc.OKResp{OK: true}
		case "copy":
			err = m.Copy(req.Username, req.Path, req.Dest, req.Root)
			out = rpc.OKResp{OK: true}
		case "extract":
			dest, e := m.Extract(req.Username, req.Path, req.Root)
			err = e
			out = rpc.FileOpResp{OK: err == nil, Dest: dest, Path: dest}
		}
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, err)
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
	writeJSON(w, status, rpc.ErrorBody{Error: err.Error()})
}

func goAccessInfo(domain string, refresh bool) rpc.GoAccessInfo {
	out := rpc.GoAccessInfo{Domain: domain, Installed: weblog.Status().GoAccess}
	if refresh {
		if err := weblog.Generate(domain); err != nil {
			out.Message = err.Error()
			return out
		}
	} else if _, err := weblog.ReadReport(domain); err != nil {
		if err := weblog.Generate(domain); err != nil {
			out.Message = err.Error()
			return out
		}
	}
	gen, size, ok := weblog.ReportInfo(domain)
	if !ok {
		out.Message = "report not found"
		return out
	}
	out.OK = true
	out.Bytes = size
	out.GeneratedAt = gen.Format("2006-01-02 15:04:05")
	return out
}

func lookupGID(name string) int {
	g, err := user.LookupGroup(name)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(g.Gid)
	return n
}

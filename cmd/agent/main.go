//go:build linux

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
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
	"github.com/siroc-dev/siroc/internal/layout"
	"github.com/siroc-dev/siroc/internal/monitoring"
	"github.com/siroc-dev/siroc/internal/osupdate"
	"github.com/siroc-dev/siroc/internal/pma"
	"github.com/siroc-dev/siroc/internal/rclone"
	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/security"
	"github.com/siroc-dev/siroc/internal/software"
	"github.com/siroc-dev/siroc/internal/sqlpack"
	"github.com/siroc-dev/siroc/internal/supervisor"
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
		case "version", "-v", "--version":
			fmt.Println(version.Current())
			return
		case "file-helper":
			files.HelperMain()
			return
		case "update", "check", "check-update":
			if os.Geteuid() != 0 {
				log.Fatal("siroc-agent must run as root")
			}
			ensureLayout()
			if err := runUpdateCheck(os.Args[2:]); err != nil {
				log.Fatal(err)
			}
			return
		case "upgrade", "apply":
			if os.Geteuid() != 0 {
				log.Fatal("siroc-agent must run as root")
			}
			ensureLayout()
			if err := runUpdateApply(os.Args[2:]); err != nil {
				log.Fatal(err)
			}
			return
		case "nginx-modules-hook":
			if os.Geteuid() != 0 {
				log.Fatal("siroc-agent must run as root")
			}
			if err := software.InstallNginxModuleHook(); err != nil {
				log.Printf("nginx module hook: %v", err)
			}
			if err := software.RecoverNginxModules(); err != nil {
				log.Printf("nginx modules: %v", err)
			}
			return
		case "nginx-modules":
			if os.Geteuid() != 0 {
				log.Fatal("siroc-agent must run as root")
			}
			if err := software.RebuildNginxModules(); err != nil {
				log.Fatal(err)
			}
			return
		case "os-check":
			if os.Geteuid() != 0 {
				log.Fatal("siroc-agent must run as root")
			}
			st, err := osupdate.Update()
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf("%s %s\n", st.CheckedAt, st.Message)
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
	ensureLayout()
	if err := sysops.RestoreManagedHosts(); err != nil {
		log.Printf("hosts: %v", err)
	}
	repairAgentUnit()
	hosting.RepairNginxHTTP2()
	hosting.RepairAssetLogs()
	if repaired, err := dbmgmt.RepairMariaDBConfig(); err != nil {
		log.Printf("mariadb config: %v", err)
	} else if repaired {
		log.Printf("removed obsolete MariaDB options so the server can start")
	}

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
	if err := software.EnsurePHPCLILimits(); err != nil {
		log.Printf("php cli limits: %v", err)
	}
	go func() {
		if err := software.EnsureNginxModules(); err != nil {
			log.Printf("nginx modules: %v", err)
		}
	}()

	usersMgr := &users.Manager{HomeRoot: cfg.HomeRoot}
	if n := usersMgr.RepairHomes(); n > 0 {
		log.Printf("recreated %d missing linux account(s) from /home", n)
	}
	if err := hosting.ReloadSupervisor(); err != nil {
		log.Printf("supervisor reload: %v", err)
	}
	filesMgr := &files.Manager{HomeRoot: cfg.HomeRoot}
	softMgr := &software.Manager{}
	hostMgr := &hosting.Manager{HomeRoot: cfg.HomeRoot, NginxSites: cfg.NginxSites, ApacheSites: cfg.ApacheSites}
	dbMgr := &dbmgmt.Manager{}
	secMgr := &security.Manager{HomeRoot: cfg.HomeRoot}
	mon := &monitoring.Collector{}
	mon.StartDisk()
	weblog.Ensure()
	if err := update.InstallCheckTimer(); err != nil {
		log.Printf("update check timer: %v", err)
	}
	if err := software.InstallSSLRenew(); err != nil {
		log.Printf("ssl renew cron: %v", err)
	}
	if err := osupdate.InstallTimer(); err != nil {
		log.Printf("os update timer: %v", err)
	}
	if err := software.InstallNginxModuleHook(); err != nil {
		log.Printf("nginx module hook: %v", err)
	}
	if err := security.EnsureWAF(); err != nil {
		log.Printf("waf config: %v", err)
	}
	if err := security.EnsureFail2ban(); err != nil {
		log.Printf("fail2ban: %v", err)
	}
	if err := security.EnsureFirewall(); err != nil {
		log.Printf("firewall: %v", err)
	}
	hostMgr.FixAllWebPerms()
	hostMgr.LockOpenBasedir()

	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, rpc.HealthResp{OK: true, Root: true, Socket: cfg.SocketPath})
	})
	r.Get("/update/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, update.Status(os.Getenv("SIROC_UPDATE_URL")))
	})
	r.Post("/update", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.PanelUpdateReq
		if !decode(w, r, &req) {
			return
		}
		channel := update.ResolveChannel(req.Channel)
		if strings.TrimSpace(req.Channel) == "" && strings.TrimSpace(os.Getenv("SIROC_UPDATE_URL")) != "" {
			channel = update.ResolveChannel(os.Getenv("SIROC_UPDATE_URL"))
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
		case "auto":
			if req.Auto == nil {
				writeErr(w, http.StatusBadRequest, fmt.Errorf("auto is required"))
				return
			}
			if err = update.SetAutoUpdate(*req.Auto); err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			out = update.Status(channel)
			if *req.Auto {
				out.Message = "Automatic updates are on"
			} else {
				out.Message = "Automatic updates are off"
			}
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
	r.Get("/os/updates", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, osupdate.Saved())
	})
	r.Post("/os/updates", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.OSUpdateReq
		if !decode(w, r, &req) {
			return
		}
		var (
			out rpc.OSUpdateStatus
			err error
		)
		switch strings.ToLower(strings.TrimSpace(req.Action)) {
		case "update":
			out, err = osupdate.Update()
		case "upgrade":
			out, err = osupdate.Upgrade()
			if err == nil {
				if rerr := software.RecoverNginxModules(); rerr != nil {
					log.Printf("nginx modules: %v", rerr)
				}
			}
		default:
			writeErr(w, http.StatusBadRequest, fmt.Errorf("action must be update or upgrade"))
			return
		}
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})

	r.Get("/users", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, []rpc.UserResp{})
	})
	r.Post("/users/ensure", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.UserEnsureReq
		if !decode(w, r, &req) {
			return
		}
		u, err := usersMgr.Ensure(req.Username, req.UID, req.GID, req.Password)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, u)
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
	r.Post("/files/archive", fileHandler(filesMgr, "archive"))
	r.Get("/files/download", func(w http.ResponseWriter, r *http.Request) {
		username := r.URL.Query().Get("username")
		path := r.URL.Query().Get("path")
		root := r.URL.Query().Get("root") == "1"
		if err := filesMgr.For(username, r.URL.Query()["roots"]).ServeDownload(w, username, path, root); err != nil {
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
		r.Body = http.MaxBytesReader(w, r.Body, files.MaxUpload+8<<20)
		_ = r.ParseMultipartForm(32 << 20)
		username := r.FormValue("username")
		path := r.FormValue("path")
		f, _, err := r.FormFile("file")
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		defer f.Close()
		if err := filesMgr.For(username, r.Form["roots"]).Upload(username, path, f, r.FormValue("root") == "1"); err != nil {
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
	r.Get("/software/log", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		var offset int64
		fmt.Sscan(r.URL.Query().Get("offset"), &offset)
		text, next, err := software.ReadInstallLog(name, offset)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.InstallLogResp{Name: name, Text: text, Offset: next})
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
	r.Post("/software/redis/conf", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.RedisConfApply
		if !decode(w, r, &req) {
			return
		}
		if err := software.ApplyRedisConf(req.Conf); err != nil {
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

	r.Post("/sites/config/read", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.SiteConfigReq
		if !decode(w, r, &req) {
			return
		}
		nginx, apache, err := hostMgr.ReadSiteConfig(req.Domain)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.SiteConfigResp{OK: true, Nginx: nginx, Apache: apache})
	})
	r.Post("/sites/config/test", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.SiteConfigReq
		if !decode(w, r, &req) {
			return
		}
		log, err := hostMgr.ApplySiteConfig(req.Domain, req.Nginx, req.Apache, req.Enabled, false)
		if err != nil {
			writeJSON(w, http.StatusOK, rpc.SiteConfigResp{Log: log, Message: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, rpc.SiteConfigResp{OK: true, Log: log})
	})
	r.Post("/sites/config/save", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.SiteConfigReq
		if !decode(w, r, &req) {
			return
		}
		log, err := hostMgr.ApplySiteConfig(req.Domain, req.Nginx, req.Apache, req.Enabled, true)
		if err != nil {
			writeJSON(w, http.StatusOK, rpc.SiteConfigResp{Log: log, Message: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, rpc.SiteConfigResp{OK: true, Log: log, Nginx: req.Nginx, Apache: req.Apache})
	})
	r.Post("/sites/rebuild", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.SitesRebuildReq
		if !decode(w, r, &req) {
			return
		}
		out, err := hostMgr.Rebuild(req.Sites)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/sites/bandwidth", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.BandwidthReq
		if !decode(w, r, &req) {
			return
		}
		sites, err := hosting.SiteBandwidth(req.Domains)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.BandwidthResp{Sites: sites})
	})
	r.Post("/sites/default", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.DefaultSiteReq
		if !decode(w, r, &req) {
			return
		}
		if err := hosting.ApplyDefaultWebsite(req.Domain); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})
	r.Post("/sites/cache/purge", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.CachePurgeReq
		if !decode(w, r, &req) {
			return
		}
		if err := hosting.PurgeNginxCache(req.Path); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
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
			if out != nil && (out.Log != "" || out.Message != "") {
				if out.Message == "" {
					out.Message = err.Error()
				}
				writeJSON(w, http.StatusOK, out)
				return
			}
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/sites/ssl/info", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.SiteSSLInfoReq
		if !decode(w, r, &req) {
			return
		}
		out, err := hostMgr.CertInfo(req)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/sites/ssl/custom", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.SiteCustomSSLReq
		if !decode(w, r, &req) {
			return
		}
		out, err := hostMgr.InstallCustomSSL(req)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/ssl/letsencrypt/account", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.LEAccountReq
		if !decode(w, r, &req) {
			return
		}
		out, err := hostMgr.RegisterLEAccount(req)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/ssl/letsencrypt/account/status", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.LEAccountReq
		if !decode(w, r, &req) {
			return
		}
		out, err := hostMgr.LEAccountStatus(req)
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
	r.Post("/git/key", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.GitKeyReq
		if !decode(w, r, &req) {
			return
		}
		out, err := hostMgr.GitKey(req.Username)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/git/branches", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.GitBranchesReq
		if !decode(w, r, &req) {
			return
		}
		out, err := hostMgr.GitBranches(req.Username, req.Repo)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/git/head", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.GitHeadReq
		if !decode(w, r, &req) {
			return
		}
		out, err := hostMgr.GitHead(req.Username, req.Path)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/git/deploy", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.GitDeployReq
		if !decode(w, r, &req) {
			return
		}
		out, err := hostMgr.GitDeploy(req)
		if out != nil {
			writeJSON(w, http.StatusOK, out)
			return
		}
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.GitDeployResp{OK: true})
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
	r.Post("/sites/move", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.SiteMoveReq
		if !decode(w, r, &req) {
			return
		}
		if err := hostMgr.MoveUser(req); err != nil {
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
	r.Post("/db/create-priv", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.DBCreatePrivReq
		if !decode(w, r, &req) {
			return
		}
		if err := dbMgr.SetCreate(req.DBUser, req.Account, req.Enabled); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})
	r.Post("/db/sync", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.DBSyncReq
		if !decode(w, r, &req) {
			return
		}
		accounts := make([]dbmgmt.SyncAccount, 0, len(req.Accounts))
		for _, acc := range req.Accounts {
			accounts = append(accounts, dbmgmt.SyncAccount{Username: acc.Username, Users: acc.Users})
		}
		found, err := dbMgr.SyncPrivileged(accounts)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		out := rpc.DBSyncResp{Found: make([]rpc.DBSyncDB, 0, len(found))}
		for _, row := range found {
			out.Found = append(out.Found, rpc.DBSyncDB{Username: row.Username, DBUser: row.DBUser, DBName: row.DBName})
		}
		writeJSON(w, http.StatusOK, out)
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
	r.Post("/security/firewall/addresses", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.FirewallAddressReq
		if !decode(w, r, &req) {
			return
		}
		if err := security.SetFirewallAddress(req.List, req.Op, req.Address); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		st, err := secMgr.FirewallStatus()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, st)
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
		out, err := secMgr.WAFRules(r.URL.Query().Get("q"), r.URL.Query().Get("pack"), 200, security.ParseWAFPin(r.URL.Query().Get("pin")))
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
	r.Post("/system/site-disk", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.SiteDiskReq
		if !decode(w, r, &req) {
			return
		}
		writeJSON(w, http.StatusOK, mon.SiteDisk(req.Home, req.Items))
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
	r.Get("/redis/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, software.RedisStatus())
	})
	r.Get("/databases/monitor", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, software.DatabasesMonitor(""))
	})
	r.Get("/logs/status", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, weblog.Status())
	})
	r.Get("/logs/system", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, weblog.SystemLogs(r.URL.Query().Get("id"), r.URL.Query().Get("group"), 0))
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
	r.Post("/logs/site", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.SiteLogReq
		if !decode(w, r, &req) {
			return
		}
		out, err := weblog.SiteLogs(req)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/logs/site/clear", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.SiteLogReq
		if !decode(w, r, &req) {
			return
		}
		out, err := weblog.ClearSiteLogs(req)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
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

	r.Get("/db/sizes", func(w http.ResponseWriter, _ *http.Request) {
		out, err := dbMgr.Sizes()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if out == nil {
			out = map[string]int64{}
		}
		writeJSON(w, http.StatusOK, out)
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
		if err := dbMgr.SetPassword(req.DBUser, req.Password, req.DBName, req.Account); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})
	r.Get("/db/export", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		format := r.URL.Query().Get("format")
		filename, contentType, err := sqlpack.ExportMeta(name, format)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		f, err := dbMgr.Dump(name)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		defer os.Remove(f.Name())
		defer f.Close()
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		switch strings.ToLower(strings.TrimSpace(format)) {
		case "gz", "gzip":
			err = sqlpack.Gzip(w, f)
		case "zip":
			err = sqlpack.Zip(w, strings.TrimSuffix(filename, ".zip")+".sql", f)
		default:
			_, err = io.Copy(w, f)
		}
		if err != nil {
			log.Printf("db export %s: %v", name, err)
		}
	})
	r.Get("/db/import-log", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		var offset int64
		fmt.Sscan(r.URL.Query().Get("offset"), &offset)
		text, next, err := dbmgmt.ReadImportLog(name, offset)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.InstallLogResp{Name: name, Text: text, Offset: next})
	})
	r.Post("/db/import", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, sqlpack.MaxUpload+8<<20)
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("upload too large or invalid (max 10GB)"))
			return
		}
		file, hdr, err := r.FormFile("file")
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		defer file.Close()
		if err := dbMgr.Import(r.FormValue("name"), r.FormValue("user"), r.FormValue("password"), hdr.Filename, file); err != nil {
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
		out, err := filesMgr.For(req.Username, req.Roots).Fetch(req.Username, req.Path, req.URL, req.Dest, req.Root)
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
		out, err := filesMgr.For(req.Username, req.Roots).Search(req.Username, req.Path, req.Query, req.Root)
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
	r.Get("/sysops/hosts", func(w http.ResponseWriter, _ *http.Request) {
		out, err := sysops.ListHostEntries()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/sysops/hosts", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.HostsReq
		if !decode(w, r, &req) {
			return
		}
		out, err := sysops.ChangeHosts("add", 0, req.IP, req.Names)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Put("/sysops/hosts/{id}", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.HostsReq
		if !decode(w, r, &req) {
			return
		}
		id, _ := strconv.Atoi(chi.URLParam(r, "id"))
		out, err := sysops.ChangeHosts("update", id, req.IP, req.Names)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Delete("/sysops/hosts/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(chi.URLParam(r, "id"))
		out, err := sysops.ChangeHosts("delete", id, "", nil)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Get("/sysops/nfs", func(w http.ResponseWriter, _ *http.Request) {
		out, err := sysops.ListNFS()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/sysops/nfs/install", func(w http.ResponseWriter, _ *http.Request) {
		out, err := sysops.InstallNFS()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/sysops/nfs/service", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Action string `json:"action"`
		}
		if !decode(w, r, &req) {
			return
		}
		out, err := sysops.ControlNFS(req.Action)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/sysops/nfs/exports", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.NFSExportReq
		if !decode(w, r, &req) {
			return
		}
		out, err := sysops.ChangeExport("add", 0, req)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Put("/sysops/nfs/exports/{id}", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.NFSExportReq
		if !decode(w, r, &req) {
			return
		}
		id, _ := strconv.Atoi(chi.URLParam(r, "id"))
		out, err := sysops.ChangeExport("update", id, req)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Delete("/sysops/nfs/exports/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(chi.URLParam(r, "id"))
		out, err := sysops.ChangeExport("delete", id, rpc.NFSExportReq{})
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/sysops/nfs/mounts", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.NFSMountReq
		if !decode(w, r, &req) {
			return
		}
		out, err := sysops.ChangeMount("add", 0, req)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Put("/sysops/nfs/mounts/{id}", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.NFSMountReq
		if !decode(w, r, &req) {
			return
		}
		id, _ := strconv.Atoi(chi.URLParam(r, "id"))
		out, err := sysops.ChangeMount("update", id, req)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Delete("/sysops/nfs/mounts/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(chi.URLParam(r, "id"))
		out, err := sysops.ChangeMount("delete", id, rpc.NFSMountReq{})
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/sysops/nfs/mounts/{id}/{action}", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(chi.URLParam(r, "id"))
		out, err := sysops.ChangeMount(chi.URLParam(r, "action"), id, rpc.NFSMountReq{})
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Get("/supervisor", func(w http.ResponseWriter, _ *http.Request) {
		out, err := supervisor.List()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/supervisor/install", func(w http.ResponseWriter, _ *http.Request) {
		out, err := supervisor.Install()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/supervisor/service", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Action string `json:"action"`
		}
		if !decode(w, r, &body) {
			return
		}
		out, err := supervisor.Service(body.Action)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/supervisor/reload", func(w http.ResponseWriter, _ *http.Request) {
		out, err := supervisor.Reload()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/supervisor/programs", func(w http.ResponseWriter, r *http.Request) {
		var body rpc.SupervisorProgramReq
		if !decode(w, r, &body) {
			return
		}
		out, err := supervisor.Create(body)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Put("/supervisor/programs/{name}", func(w http.ResponseWriter, r *http.Request) {
		var body rpc.SupervisorProgramReq
		if !decode(w, r, &body) {
			return
		}
		out, err := supervisor.Update(chi.URLParam(r, "name"), body)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Delete("/supervisor/programs/{name}", func(w http.ResponseWriter, r *http.Request) {
		out, err := supervisor.Delete(chi.URLParam(r, "name"))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/supervisor/programs/{name}/{action}", func(w http.ResponseWriter, r *http.Request) {
		out, err := supervisor.Ctl(chi.URLParam(r, "name"), chi.URLParam(r, "action"))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Get("/supervisor/programs/{name}/log", func(w http.ResponseWriter, r *http.Request) {
		text, err := supervisor.Log(chi.URLParam(r, "name"))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"text": text})
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
	r.Post("/backup/cron", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.BackupCronFile
		if !decode(w, r, &req) {
			return
		}
		if err := backup.InstallCron(req.Body); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})
	r.Get("/rclone", func(w http.ResponseWriter, r *http.Request) {
		out, err := rclone.Status()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/rclone/remotes", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.RcloneRemoteReq
		if !decode(w, r, &req) {
			return
		}
		if err := rclone.Create(req); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})
	r.Post("/rclone/remotes/update", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.RcloneRemoteReq
		if !decode(w, r, &req) {
			return
		}
		if err := rclone.Update(req); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})
	r.Post("/rclone/remotes/delete", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.RcloneRemoteReq
		if !decode(w, r, &req) {
			return
		}
		if err := rclone.Delete(req.Name); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
	})
	r.Post("/rclone/list", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.RcloneListReq
		if !decode(w, r, &req) {
			return
		}
		out, err := rclone.List(req)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/rclone/run", func(w http.ResponseWriter, r *http.Request) {
		var req rpc.RcloneRunReq
		if !decode(w, r, &req) {
			return
		}
		out, err := rclone.Start(req)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Get("/rclone/run", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, rclone.Job())
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
	r.Post("/backup/upload", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, backup.MaxRestoreUpload+8<<20)
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("upload too large or invalid (max 10GB)"))
			return
		}
		file, hdr, err := r.FormFile("file")
		if err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("file is required"))
			return
		}
		defer file.Close()
		path, err := backup.SaveUpload(hdr.Filename, file)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		out, err := backup.Run(rpc.BackupReq{Restore: path})
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/pma/ensure", func(w http.ResponseWriter, _ *http.Request) {
		if err := pma.Ensure(); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, rpc.OKResp{OK: true})
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
		mgr := m.For(req.Username, req.Roots)
		var (
			out any
			err error
		)
		switch op {
		case "list":
			out, err = mgr.List(req.Username, req.Path, req.Root)
		case "read":
			out, err = mgr.Read(req.Username, req.Path, req.Root)
		case "write":
			err = mgr.Write(req.Username, req.Path, req.Content, req.Root)
			out = rpc.OKResp{OK: true}
		case "mkdir":
			err = mgr.Mkdir(req.Username, req.Path, req.Root)
			out = rpc.OKResp{OK: true}
		case "delete":
			err = mgr.Delete(req.Username, req.Path, req.Root)
			out = rpc.OKResp{OK: true}
		case "rename":
			err = mgr.Rename(req.Username, req.Path, req.Dest, req.Root)
			out = rpc.OKResp{OK: true}
		case "chmod":
			err = mgr.Chmod(req.Username, req.Path, req.Mode, req.Root)
			out = rpc.OKResp{OK: true}
		case "copy":
			err = mgr.Copy(req.Username, req.Path, req.Dest, req.Root)
			out = rpc.OKResp{OK: true}
		case "extract":
			dest, e := mgr.Extract(req.Username, req.Path, req.Root)
			err = e
			out = rpc.FileOpResp{OK: err == nil, Dest: dest, Path: dest}
		case "archive":
			dest, e := mgr.Archive(req.Username, req.Path, req.Root)
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

func ensureLayout() {
	if err := layout.Migrate(); err != nil {
		log.Printf("siroc layout: %v", err)
	}
}

func repairAgentUnit() {
	const path = "/etc/systemd/system/siroc-agent.service"
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	next, changed := update.StripRuntimeDirectoryGroup(string(b))
	if withCloud, cloudChanged := update.AfterCloudInit(next); cloudChanged {
		next = withCloud
		changed = true
	}
	if !changed {
		return
	}
	if err := os.WriteFile(path, []byte(next), 0644); err != nil {
		log.Printf("agent unit: %v", err)
		return
	}
	if out, err := exec.Command("systemctl", "daemon-reload").CombinedOutput(); err != nil {
		log.Printf("daemon-reload: %s", strings.TrimSpace(string(out)))
	}
}

func lookupGID(name string) int {
	g, err := user.LookupGroup(name)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(g.Gid)
	return n
}

func defaultUpdateChannel() string {
	return update.ResolveChannel(os.Getenv("SIROC_UPDATE_URL"))
}

func parseUpdateArgs(args []string) (channel, srcURL, srcPath string) {
	channel = defaultUpdateChannel()
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--channel" && i+1 < len(args):
			i++
			channel = strings.TrimSpace(args[i])
		case a == "--url" && i+1 < len(args):
			i++
			srcURL = strings.TrimSpace(args[i])
		case a == "--path" && i+1 < len(args):
			i++
			srcPath = strings.TrimSpace(args[i])
		case strings.HasPrefix(a, "-"):
			continue
		default:
			channel = strings.TrimSpace(a)
		}
	}
	return channel, srcURL, srcPath
}

func printUpdateStatus(st *rpc.PanelUpdateStatus) {
	if st == nil {
		return
	}
	fmt.Printf("Installed: %s\n", st.Version)
	if st.Latest != "" {
		fmt.Printf("Latest:    %s\n", st.Latest)
	}
	if st.Channel != "" {
		fmt.Printf("Channel:   %s\n", st.Channel)
	}
	if st.PackageURL != "" {
		fmt.Printf("Package:   %s\n", st.PackageURL)
	}
	if st.Message != "" {
		fmt.Println(st.Message)
	}
}

func runUpdateCheck(args []string) error {
	channel, _, _ := parseUpdateArgs(args)
	var st *rpc.PanelUpdateStatus
	var err error
	if updateApplyFlag(args) {
		st, err = update.CheckApply(channel)
	} else {
		st, err = update.Check(channel)
	}
	if err != nil {
		return err
	}
	printUpdateStatus(st)
	return nil
}

func updateApplyFlag(args []string) bool {
	for _, a := range args {
		if a == "--apply" {
			return true
		}
	}
	return false
}

func runUpdateApply(args []string) error {
	channel, srcURL, srcPath := parseUpdateArgs(args)
	if srcURL == "" && srcPath == "" {
		if _, err := update.Check(channel); err != nil {
			return err
		}
	}
	st, err := update.Apply(channel, srcURL, srcPath)
	if err != nil {
		return err
	}
	printUpdateStatus(st)
	return nil
}

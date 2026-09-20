package main

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/siroc-dev/siroc/internal/admincli"
	"github.com/siroc-dev/siroc/internal/api"
	"github.com/siroc-dev/siroc/internal/auth"
	"github.com/siroc-dev/siroc/internal/config"
	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/store"
	"github.com/siroc-dev/siroc/internal/version"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "-v", "--version":
			fmt.Println(version.Current())
			return
		case "passwd-admin", "reset-admin":
			if err := runPasswdAdmin(os.Args[2:]); err != nil {
				log.Fatal(err)
			}
			return
		}
	}
	log.Printf("siroc-panel %s", version.Current())
	cfg := config.Load()
	if err := os.MkdirAll(cfg.DataDir, 0750); err != nil {
		log.Fatal(err)
	}
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	var static fs.FS
	webDir := os.Getenv("SIROC_WEB_DIR")
	if webDir == "" {
		webDir = filepath.Join(cfg.InstallRoot, "web", "dist")
	}
	if _, err := os.Stat(filepath.Join(webDir, "index.html")); err == nil {
		static = os.DirFS(webDir)
	}

	srv := &api.Server{
		Cfg:    cfg,
		Store:  st,
		Auth:   auth.New(st),
		Agent:  rpc.NewClient(cfg.SocketPath),
		Static: static,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func runPasswdAdmin(args []string) error {
	cfg := config.Load()
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()
	var username, password string
	if len(args) > 0 {
		username = args[0]
	}
	if len(args) > 1 {
		password = args[1]
	}
	user, pass, err := admincli.ResetAdmin(st, username, password)
	if err != nil {
		return err
	}
	fmt.Printf("Admin password reset for %s\n", user)
	fmt.Printf("New password: %s\n", pass)
	fmt.Println("Existing sessions for this admin were signed out.")
	return nil
}

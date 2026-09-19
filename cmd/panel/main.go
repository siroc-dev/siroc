package main

import (
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/siroc-dev/siroc/internal/api"
	"github.com/siroc-dev/siroc/internal/auth"
	"github.com/siroc-dev/siroc/internal/config"
	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/store"
	"github.com/siroc-dev/siroc/internal/version"
)

func main() {
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

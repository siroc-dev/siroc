//go:build linux

package main

import (
	"fmt"
	"time"

	"github.com/siroc-dev/siroc/internal/config"
	"github.com/siroc-dev/siroc/internal/panelcert"
	"github.com/siroc-dev/siroc/internal/rpc"
)

func panelCertPaths(req rpc.PanelCertReq) panelcert.Paths {
	cfg := config.Load()
	if req.DataDir == "" {
		req.DataDir = cfg.DataDir
	}
	if req.CertFile == "" {
		req.CertFile = cfg.TLSCert
	}
	if req.KeyFile == "" {
		req.KeyFile = cfg.TLSKey
	}
	return panelcert.Paths{DataDir: req.DataDir, CertFile: req.CertFile, KeyFile: req.KeyFile}
}

func panelCertView(st panelcert.Status) rpc.PanelCertStatus {
	out := rpc.PanelCertStatus{
		IP:          st.IP,
		Email:       st.Email,
		Issuer:      st.Issuer,
		IPs:         st.IPs,
		CoversIP:    st.CoversIP,
		LetsEncrypt: st.LetsEncrypt,
		Message:     st.Message,
	}
	if !st.NotAfter.IsZero() {
		out.NotAfter = st.NotAfter.UTC().Format(time.RFC3339)
	}
	return out
}

func panelCertStatus(req rpc.PanelCertReq) rpc.PanelCertStatus {
	paths := panelCertPaths(req)
	return panelCertView(panelcert.Describe(paths.CertFile))
}

func issuePanelCert(req rpc.PanelCertReq) (rpc.PanelCertStatus, error) {
	paths := panelCertPaths(req)
	if err := panelcert.InstallRenewTimer(paths.DataDir); err != nil {
		fmt.Printf("panel certificate timer: %v\n", err)
	}
	st, err := panelcert.Ensure("", paths)
	if err != nil {
		return panelCertView(st), err
	}
	return panelCertView(st), nil
}

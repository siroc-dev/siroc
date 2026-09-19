package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/validate"
)

func (s *Server) leSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.loadLEConfig())
}

func (s *Server) setLESettings(w http.ResponseWriter, r *http.Request) {
	var body rpc.LEConfig
	if !decode(w, r, &body) {
		return
	}
	cfg, err := normalizeLEConfig(body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := validate.Email(cfg.Email); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	curHmac, _ := s.Store.Setting("le_eab_hmac")
	hmac := strings.TrimSpace(body.EABHMAC)
	if hmac == "" || hmac == "********" {
		hmac = curHmac
	}
	if strings.TrimSpace(cfg.EABKID) == "" {
		hmac = ""
	}
	_ = s.Store.SetSetting("le_email", cfg.Email)
	_ = s.Store.SetSetting("le_server", cfg.Server)
	_ = s.Store.SetSetting("le_directory", cfg.Directory)
	_ = s.Store.SetSetting("le_key_type", cfg.KeyType)
	_ = s.Store.SetSetting("le_rsa_key_size", strconv.Itoa(cfg.RSAKeySize))
	_ = s.Store.SetSetting("le_eab_kid", cfg.EABKID)
	_ = s.Store.SetSetting("le_eab_hmac", hmac)
	if cfg.NoVerify {
		_ = s.Store.SetSetting("le_no_verify", "1")
	} else {
		_ = s.Store.SetSetting("le_no_verify", "0")
	}
	writeJSON(w, http.StatusOK, s.loadLEConfig())
}

func (s *Server) loadLEConfig() rpc.LEConfig {
	email, _ := s.Store.Setting("le_email")
	server, _ := s.Store.Setting("le_server")
	dir, _ := s.Store.Setting("le_directory")
	keyType, _ := s.Store.Setting("le_key_type")
	sizeStr, _ := s.Store.Setting("le_rsa_key_size")
	kid, _ := s.Store.Setting("le_eab_kid")
	hmac, _ := s.Store.Setting("le_eab_hmac")
	noVerify, _ := s.Store.Setting("le_no_verify")
	size, _ := strconv.Atoi(sizeStr)
	cfg := rpc.LEConfig{
		Email:      email,
		Server:     server,
		Directory:  dir,
		KeyType:    keyType,
		RSAKeySize: size,
		EABKID:     kid,
		EABHMAC:    hmac,
		NoVerify:   noVerify == "1",
	}
	out, _ := normalizeLEConfig(cfg)
	out.EABHMAC = hmac
	out.HasEABHMAC = strings.TrimSpace(hmac) != ""
	public := out
	public.EABHMAC = ""
	return public
}

func (s *Server) siteSSLReq(username, domain string, aliases []string, email string) rpc.SiteSSLReq {
	cfg := s.loadLEConfig()
	hmac, _ := s.Store.Setting("le_eab_hmac")
	if strings.TrimSpace(email) == "" {
		email = cfg.Email
	}
	return rpc.SiteSSLReq{
		Username:   username,
		Domain:     domain,
		Aliases:    aliases,
		Email:      email,
		Server:     cfg.Server,
		Directory:  cfg.Directory,
		KeyType:    cfg.KeyType,
		RSAKeySize: cfg.RSAKeySize,
		EABKID:     cfg.EABKID,
		EABHMAC:    hmac,
		NoVerify:   cfg.NoVerify,
	}
}

func normalizeLEConfig(in rpc.LEConfig) (rpc.LEConfig, error) {
	out := in
	out.Email = strings.TrimSpace(in.Email)
	out.Directory = strings.TrimSpace(in.Directory)
	out.EABKID = strings.TrimSpace(in.EABKID)
	switch strings.ToLower(strings.TrimSpace(in.Server)) {
	case "", "production", "prod", "letsencrypt":
		out.Server = "production"
	case "staging":
		out.Server = "staging"
	case "custom":
		out.Server = "custom"
		if out.Directory == "" {
			return out, fmt.Errorf("custom ACME directory URL is required")
		}
		u, err := url.Parse(out.Directory)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
			return out, fmt.Errorf("invalid ACME directory URL")
		}
	default:
		return out, fmt.Errorf("invalid ACME server")
	}
	if out.Server != "custom" {
		out.NoVerify = false
	}
	switch strings.ToLower(strings.TrimSpace(in.KeyType)) {
	case "", "ecdsa", "ecdsa-p256":
		out.KeyType = "ecdsa"
		out.RSAKeySize = 0
	case "ecdsa-p384":
		out.KeyType = "ecdsa-p384"
		out.RSAKeySize = 0
	case "rsa", "rsa2048":
		out.KeyType = "rsa"
		out.RSAKeySize = 2048
	case "rsa4096":
		out.KeyType = "rsa"
		out.RSAKeySize = 4096
	default:
		return out, fmt.Errorf("invalid ACME key type")
	}
	if out.KeyType == "rsa" && in.RSAKeySize == 4096 {
		out.RSAKeySize = 4096
	}
	return out, nil
}

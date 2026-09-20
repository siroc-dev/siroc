package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/siroc-dev/siroc/internal/auth"
	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/secret"
	"github.com/siroc-dev/siroc/internal/store"
)

func isAdmin(u *store.PanelUser) bool {
	return u != nil && u.Role == "admin" && u.Impersonator == ""
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isAdmin(currentUser(r)) {
			writeErr(w, http.StatusForbidden, fmt.Errorf("admin only"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) allowAccount(w http.ResponseWriter, r *http.Request, username string) bool {
	u := currentUser(r)
	if isAdmin(u) {
		return true
	}
	if u != nil && u.Username == username {
		return true
	}
	writeErr(w, http.StatusForbidden, fmt.Errorf("forbidden"))
	return false
}

func (s *Server) encryptPassword(plain string) (string, error) {
	key, err := s.Store.SecretKey()
	if err != nil {
		return "", err
	}
	return secret.Encrypt(key, plain)
}

func (s *Server) decryptPassword(enc string) (string, error) {
	key, err := s.Store.SecretKey()
	if err != nil {
		return "", err
	}
	return secret.Decrypt(key, enc)
}

func (s *Server) suggestPassword(w http.ResponseWriter, _ *http.Request) {
	pw, err := secret.RandomPassword(16)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"password": pw})
}

func (s *Server) showAccountPassword(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(currentUser(r)) {
		writeErr(w, http.StatusForbidden, fmt.Errorf("admin only"))
		return
	}
	username := chi.URLParam(r, "username")
	acc, ok := s.accountOrErr(w, r, username)
	if !ok {
		return
	}
	if acc.PasswordEnc == "" {
		writeErr(w, http.StatusNotFound, fmt.Errorf("password was not stored for this account"))
		return
	}
	pw, err := s.decryptPassword(acc.PasswordEnc)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"username": acc.Username, "password": pw})
}

func (s *Server) setAccountPassword(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	if !s.allowAccount(w, r, username) {
		return
	}
	acc, ok := s.accountOrErr(w, r, username)
	if !ok {
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !auth.ValidPassword(body.Password) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("password must be at least 8 characters"))
		return
	}
	if err := s.Agent.UserPassword(acc.Username, body.Password); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	enc, err := s.encryptPassword(body.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.Store.UpdateAccountPassword(acc.Username, enc); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	hash, err := auth.Hash(body.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.Store.UpsertHostPanelUser(acc.Username, hash); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "username": acc.Username, "password": body.Password})
}

func (s *Server) setAccountAccess(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(currentUser(r)) {
		writeErr(w, http.StatusForbidden, fmt.Errorf("admin only"))
		return
	}
	username := chi.URLParam(r, "username")
	acc, ok := s.accountOrErr(w, r, username)
	if !ok {
		return
	}
	var body struct {
		SSH bool `json:"ssh"`
		FTP bool `json:"ftp"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := s.Agent.UserAccess(acc.Username, body.SSH, body.FTP); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.Store.UpdateAccountAccess(acc.Username, body.SSH, body.FTP); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	acc, _ = s.Store.GetAccountByName(username)
	writeJSON(w, http.StatusOK, acc)
}

func (s *Server) loginAs(w http.ResponseWriter, r *http.Request) {
	admin := currentUser(r)
	if !isAdmin(admin) {
		writeErr(w, http.StatusForbidden, fmt.Errorf("admin only"))
		return
	}
	username := chi.URLParam(r, "username")
	acc, ok := s.accountOrErr(w, r, username)
	if !ok {
		return
	}
	if acc.Suspended {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("account is suspended"))
		return
	}
	target, err := s.Store.GetPanelUserByName(acc.Username)
	if err != nil || target.Role == "admin" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("set a password for this account first"))
		return
	}
	adminSid := auth.SessionID(r)
	sid, err := s.Store.CreateSession(target.ID, admin.ID, 7*24*time.Hour)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	secure := !s.Cfg.DisableTLS
	s.Auth.SetReturnCookie(w, adminSid, secure)
	s.Auth.SetCookie(w, sid, secure)
	writeJSON(w, http.StatusOK, map[string]any{"username": target.Username, "role": target.Role, "impersonator": admin.Username})
}

func (s *Server) returnAdmin(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil || u.Impersonator == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("not impersonating"))
		return
	}
	ret := auth.ReturnSessionID(r)
	if ret == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("admin session expired; sign in again"))
		return
	}
	admin, err := s.Store.GetSessionUser(ret)
	if err != nil || admin.Role != "admin" {
		writeErr(w, http.StatusUnauthorized, fmt.Errorf("admin session expired; sign in again"))
		return
	}
	if sid := auth.SessionID(r); sid != "" {
		_ = s.Store.DeleteSession(sid)
	}
	secure := !s.Cfg.DisableTLS
	s.Auth.SetCookie(w, ret, secure)
	s.Auth.ClearReturnCookie(w, secure)
	writeJSON(w, http.StatusOK, map[string]any{"username": admin.Username, "role": admin.Role})
}

func (s *Server) listFTP(w http.ResponseWriter, r *http.Request) {
	accountID := int64(0)
	if name := r.URL.Query().Get("user"); name != "" {
		acc, ok := s.accountOrErr(w, r, name)
		if !ok {
			return
		}
		accountID = acc.ID
	} else if !isAdmin(currentUser(r)) {
		acc, err := s.Store.GetAccountByName(currentUser(r).Username)
		if err != nil {
			writeJSON(w, http.StatusOK, []store.FTPUser{})
			return
		}
		accountID = acc.ID
	}
	list, err := s.Store.ListFTPUsers(accountID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createFTP(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Name     string `json:"name"`
		Password string `json:"password"`
		Home     string `json:"home"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !isAdmin(currentUser(r)) {
		body.Username = currentUser(r).Username
	}
	acc, ok := s.accountOrErr(w, r, body.Username)
	if !ok {
		return
	}
	if !s.allowAccount(w, r, acc.Username) {
		return
	}
	if !auth.ValidPassword(body.Password) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("password must be at least 8 characters"))
		return
	}
	created, err := s.Agent.FTPCreate(rpc.FTPCreateReq{Owner: acc.Username, Name: body.Name, Password: body.Password, Home: body.Home})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	enc, err := s.encryptPassword(body.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	row, err := s.Store.CreateFTPUser(acc.ID, created.Login, created.Home, enc)
	if err != nil {
		_ = s.Agent.FTPDelete(created.Login)
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": row, "password": body.Password})
}

func (s *Server) showFTPPassword(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(currentUser(r)) {
		writeErr(w, http.StatusForbidden, fmt.Errorf("admin only"))
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid id"))
		return
	}
	row, err := s.Store.GetFTPUser(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, fmt.Errorf("ftp user not found"))
		return
	}
	if row.PasswordEnc == "" {
		writeErr(w, http.StatusNotFound, fmt.Errorf("password was not stored"))
		return
	}
	pw, err := s.decryptPassword(row.PasswordEnc)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"login": row.Login, "password": pw})
}

func (s *Server) setFTPPassword(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid id"))
		return
	}
	row, err := s.Store.GetFTPUser(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, fmt.Errorf("ftp user not found"))
		return
	}
	if !s.allowAccount(w, r, row.Username) {
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !auth.ValidPassword(body.Password) {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("password must be at least 8 characters"))
		return
	}
	if err := s.Agent.FTPPassword(row.Login, body.Password); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	enc, err := s.encryptPassword(body.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	_ = s.Store.UpdateFTPPassword(row.ID, enc)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) deleteFTP(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid id"))
		return
	}
	row, err := s.Store.GetFTPUser(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, fmt.Errorf("ftp user not found"))
		return
	}
	if !s.allowAccount(w, r, row.Username) {
		return
	}
	if err := s.Agent.FTPDelete(row.Login); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = s.Store.DeleteFTPUser(row.ID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) removeAccountFTP(username string) {
	acc, err := s.Store.GetAccountByName(username)
	if err != nil {
		return
	}
	list, _ := s.Store.ListFTPUsers(acc.ID)
	for _, f := range list {
		_ = s.Agent.FTPDelete(f.Login)
		_ = s.Store.DeleteFTPUser(f.ID)
	}
}

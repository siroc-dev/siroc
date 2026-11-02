package api

import (
	"fmt"
	"net/http"

	"github.com/siroc-dev/siroc/internal/dbmgmt"
	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/store"
	"github.com/siroc-dev/siroc/internal/validate"
)

func (s *Server) syncDatabases(w http.ResponseWriter, r *http.Request) {
	engine, err := s.Agent.DBEngine()
	if err != nil || engine == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("MySQL/MariaDB is not installed"))
		return
	}
	accounts, err := s.Store.ListAccounts()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	dbs, err := s.Store.ListDatabases()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	me := currentUser(r)
	admin := isAdmin(me)
	usersByAccount := map[string][]string{}
	passByUser := map[string]string{}
	known := map[string]struct{}{}
	for _, d := range dbs {
		if !admin && d.Username != me.Username {
			continue
		}
		known[d.DBName] = struct{}{}
		if !d.CanCreate {
			continue
		}
		if !hasString(usersByAccount[d.Username], d.DBUser) {
			usersByAccount[d.Username] = append(usersByAccount[d.Username], d.DBUser)
		}
		if d.PasswordEnc != "" && passByUser[d.DBUser] == "" {
			passByUser[d.DBUser] = d.PasswordEnc
		}
	}
	accByName := map[string]store.Account{}
	req := make([]rpc.DBSyncAccount, 0, len(accounts))
	for _, acc := range accounts {
		if !admin && acc.Username != me.Username {
			continue
		}
		accByName[acc.Username] = acc
		users := usersByAccount[acc.Username]
		if len(users) == 0 {
			continue
		}
		req = append(req, rpc.DBSyncAccount{Username: acc.Username, Users: users})
	}
	if len(req) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"added": 0})
		return
	}
	found, err := s.Agent.DBSync(req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	added := 0
	if found != nil {
		for _, row := range found.Found {
			if _, ok := known[row.DBName]; ok {
				continue
			}
			acc, ok := accByName[row.Username]
			if !ok || (!admin && acc.Username != me.Username) {
				continue
			}
			if err := validate.DBIdent(row.DBName); err != nil {
				continue
			}
			if err := validate.DBIdent(row.DBUser); err != nil {
				continue
			}
			if _, err := s.Store.GetDatabaseByName(row.DBName); err == nil {
				continue
			}
			created, err := s.Store.CreateDatabase(acc.ID, row.DBName, row.DBUser, engine, passByUser[row.DBUser])
			if err != nil {
				continue
			}
			if created.Access != dbmgmt.AccessLocal {
				_ = s.Agent.DBGrant(created.DBUser, created.DBName, dbmgmt.ExpandStored(created.AccessHosts))
			}
			known[row.DBName] = struct{}{}
			_ = s.Store.SetDatabaseCreate(acc.ID, row.DBUser, true)
			added++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"added": added})
}

func hasString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func (s *Server) setDatabaseCreate(w http.ResponseWriter, r *http.Request) {
	db, ok := s.databaseForRequest(w, r)
	if !ok {
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := s.Agent.DBCreatePriv(db.DBUser, db.Username, body.Enabled, dbmgmt.ExpandStored(db.AccessHosts)); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.Store.SetDatabaseCreate(db.AccountID, db.DBUser, body.Enabled); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": body.Enabled})
}

func (s *Server) setDatabaseAccess(w http.ResponseWriter, r *http.Request) {
	db, ok := s.databaseForRequest(w, r)
	if !ok {
		return
	}
	var body struct {
		Access string   `json:"access"`
		Hosts  []string `json:"hosts"`
	}
	if !decode(w, r, &body) {
		return
	}
	if db.PasswordEnc == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("reset the password before changing which IPs can connect"))
		return
	}
	pass, err := s.decryptPassword(db.PasswordEnc)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	stored, hosts, err := dbmgmt.ResolveAccess(body.Access, body.Hosts)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	rows, err := s.Store.ListDatabases()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	var names []string
	can := false
	for _, row := range rows {
		if row.AccountID != db.AccountID || row.DBUser != db.DBUser {
			continue
		}
		names = append(names, row.DBName)
		if row.CanCreate {
			can = true
		}
	}
	if len(names) == 0 {
		names = []string{db.DBName}
	}
	if err := s.Agent.DBAccess(rpc.DBAccessReq{
		DBUser:    db.DBUser,
		Password:  pass,
		Account:   db.Username,
		DBNames:   names,
		Hosts:     hosts,
		OldHosts:  dbmgmt.ExpandStored(db.AccessHosts),
		CanCreate: can,
	}); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.Store.SetDatabaseAccess(db.AccountID, db.DBUser, stored); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	access, list := dbmgmt.DescribeStored(stored)
	writeJSON(w, http.StatusOK, map[string]any{"access": access, "hosts": list})
}

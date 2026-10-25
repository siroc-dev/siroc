//go:build linux

package hosting

import (
	"fmt"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func attachWPUsers(st *rpc.WPStatus, user, root string, refresh bool) error {
	if st == nil {
		return nil
	}
	now := time.Now()
	if !refresh {
		if c, ok := loadWPUserCache(user, root, now); ok {
			st.Users = c.Users
			st.UsersAt = c.At.UTC().Format(time.RFC3339)
			return nil
		}
	}
	users, err := fetchWPStaff(user, root)
	if err != nil {
		if c, ok := readWPUserCache(user, root); ok && !refresh {
			st.Users = c.Users
			st.UsersAt = c.At.UTC().Format(time.RFC3339)
			return nil
		}
		st.Users = []rpc.WPUser{}
		return err
	}
	at := now.UTC()
	_ = saveWPUserCache(user, root, at, users)
	st.Users = users
	st.UsersAt = at.Format(time.RFC3339)
	return nil
}

func wpLoginTaken(user, root, login string, except int64) (bool, error) {
	out, err := wpRun(user, root, "user", "get", shellQuote(login), "--field=ID")
	if err != nil {
		low := strings.ToLower(out + " " + err.Error())
		if strings.Contains(low, "invalid user") || strings.Contains(low, "could not find") {
			return false, nil
		}
		return false, err
	}
	id := toInt64(strings.TrimSpace(out))
	return id > 0 && id != except, nil
}

func refreshWPStaff(user, root string) error {
	users, err := fetchWPStaff(user, root)
	if err != nil {
		return err
	}
	if err := saveWPUserCache(user, root, time.Now().UTC(), users); err != nil {
		return err
	}
	return nil
}

func fetchWPStaff(user, root string) ([]rpc.WPUser, error) {
	rows, err := wpStaffRows(user, root, "--role__in=administrator,editor")
	if err != nil {
		admins, aerr := wpStaffRows(user, root, "--role=administrator")
		editors, eerr := wpStaffRows(user, root, "--role=editor")
		if aerr != nil && eerr != nil {
			return nil, err
		}
		rows = append(admins, editors...)
	}
	out := make([]rpc.WPUser, 0, len(rows))
	seen := map[int64]struct{}{}
	for _, u := range rows {
		roles := wpRoleText(u["roles"])
		if !wpStaffRole(roles) {
			continue
		}
		id := toInt64(u["ID"])
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		login := fmt.Sprint(u["user_login"])
		email := fmt.Sprint(u["user_email"])
		name := fmt.Sprint(u["display_name"])
		if login == "<nil>" {
			login = ""
		}
		if email == "<nil>" {
			email = ""
		}
		if name == "<nil>" {
			name = ""
		}
		out = append(out, rpc.WPUser{
			ID:    id,
			Login: login,
			Email: email,
			Name:  name,
			Roles: roles,
		})
	}
	return out, nil
}

func wpStaffRows(user, root, roleArg string) ([]map[string]any, error) {
	var rows []map[string]any
	err := wpJSON(user, root, &rows,
		"user", "list", roleArg,
		"--fields=ID,user_login,user_email,display_name,roles",
		"--format=json",
	)
	return rows, err
}

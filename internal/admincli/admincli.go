package admincli

import (
	"fmt"
	"strings"

	"github.com/siroc-dev/siroc/internal/auth"
	"github.com/siroc-dev/siroc/internal/secret"
	"github.com/siroc-dev/siroc/internal/store"
)

func ResetAdmin(st *store.Store, username, password string) (user, pass string, err error) {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	var target *store.PanelUser
	if username == "" {
		admins, lerr := st.ListPanelUsersByRole("admin")
		if lerr != nil {
			return "", "", lerr
		}
		if len(admins) == 0 {
			return "", "", fmt.Errorf("no panel administrator exists yet")
		}
		target = &admins[0]
	} else {
		u, gerr := st.GetPanelUserByName(username)
		if gerr != nil {
			return "", "", fmt.Errorf("user %q not found", username)
		}
		if u.Role != "admin" {
			return "", "", fmt.Errorf("%q is not a panel administrator", username)
		}
		target = u
	}
	if password == "" {
		password, err = secret.RandomPassword(16)
		if err != nil {
			return "", "", err
		}
	} else if !auth.ValidPassword(password) {
		return "", "", fmt.Errorf("password must be at least 8 characters")
	}
	hash, err := auth.Hash(password)
	if err != nil {
		return "", "", err
	}
	if err := st.UpdatePassword(target.ID, hash); err != nil {
		return "", "", err
	}
	_ = st.DeleteUserSessions(target.ID)
	return target.Username, password, nil
}

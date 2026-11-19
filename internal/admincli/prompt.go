package admincli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/siroc-dev/siroc/internal/auth"
	"github.com/siroc-dev/siroc/internal/store"
)

// ErrAborted means the operator declined the reset before any password changed.
var ErrAborted = errors.New("cancelled")

// PasswordFunc reads one password without echoing it. The prompt is written separately.
type PasswordFunc func() (string, error)

// ResetInteractive confirms the administrator, then reads a hidden password.
// An empty password generates one. Nothing is changed when the operator declines.
func ResetInteractive(st *store.Store, username string, in io.Reader, out io.Writer, readPW PasswordFunc) (user, pass string, err error) {
	username = strings.TrimSpace(username)
	admins, err := st.ListPanelUsersByRole("admin")
	if err != nil {
		return "", "", err
	}

	target := username
	if target == "" {
		if len(admins) == 0 {
			return "", "", fmt.Errorf("no panel administrator exists yet")
		}
		if len(admins) == 1 {
			target = admins[0].Username
			fmt.Fprintf(out, "Admin: %s\n", target)
			ok, cerr := confirm(in, out, fmt.Sprintf("Reset password for %s? [y/N] ", target))
			if cerr != nil {
				return "", "", cerr
			}
			if !ok {
				return "", "", ErrAborted
			}
		} else {
			fmt.Fprintln(out, "Administrators:")
			for _, a := range admins {
				fmt.Fprintf(out, "  %s\n", a.Username)
			}
			fmt.Fprint(out, "Admin username: ")
			name, rerr := readLine(in)
			if rerr != nil {
				if errors.Is(rerr, io.EOF) {
					return "", "", ErrAborted
				}
				return "", "", rerr
			}
			name = strings.TrimSpace(name)
			if name == "" {
				return "", "", ErrAborted
			}
			if !adminNamed(admins, name) {
				return "", "", fmt.Errorf("%q is not a panel administrator", name)
			}
			target = name
		}
	} else {
		if !adminNamed(admins, target) {
			u, gerr := st.GetPanelUserByName(target)
			if gerr != nil {
				return "", "", fmt.Errorf("user %q not found", target)
			}
			if u.Role != "admin" {
				return "", "", fmt.Errorf("%q is not a panel administrator", target)
			}
		}
		fmt.Fprintf(out, "Admin: %s\n", target)
		ok, cerr := confirm(in, out, fmt.Sprintf("Reset password for %s? [y/N] ", target))
		if cerr != nil {
			return "", "", cerr
		}
		if !ok {
			return "", "", ErrAborted
		}
	}

	pw, err := askSecret(out, readPW, "New password (empty to generate): ")
	if err != nil {
		return "", "", err
	}
	if pw != "" {
		if !auth.ValidPassword(pw) {
			return "", "", fmt.Errorf("password must be at least 8 characters")
		}
		again, aerr := askSecret(out, readPW, "Confirm password: ")
		if aerr != nil {
			return "", "", aerr
		}
		if again != pw {
			return "", "", fmt.Errorf("passwords do not match")
		}
	}
	return ResetAdmin(st, target, pw)
}

func adminNamed(admins []store.PanelUser, name string) bool {
	for _, a := range admins {
		if a.Username == name {
			return true
		}
	}
	return false
}

func confirm(in io.Reader, out io.Writer, prompt string) (bool, error) {
	fmt.Fprint(out, prompt)
	line, err := readLine(in)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func askSecret(out io.Writer, readPW PasswordFunc, prompt string) (string, error) {
	fmt.Fprint(out, prompt)
	pw, err := readPW()
	fmt.Fprintln(out)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(pw), nil
}

func readLine(in io.Reader) (string, error) {
	var buf bytes.Buffer
	tmp := make([]byte, 1)
	for {
		n, err := in.Read(tmp)
		if n == 1 {
			if tmp[0] == '\n' {
				return buf.String(), nil
			}
			if tmp[0] != '\r' {
				buf.WriteByte(tmp[0])
			}
			if err == io.EOF {
				return buf.String(), nil
			}
			if err != nil {
				return buf.String(), err
			}
			continue
		}
		if err != nil {
			if err == io.EOF && buf.Len() > 0 {
				return buf.String(), nil
			}
			return "", err
		}
	}
}

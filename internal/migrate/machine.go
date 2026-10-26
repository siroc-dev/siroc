package migrate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
)

// Machine is the DirectAdmin server remembered by the panel.
// Password, PrivateKey, and KeyPassphrase are stored encrypted by the caller.
type Machine struct {
	Mode          string `json:"mode,omitempty"`
	Host          string `json:"host,omitempty"`
	Port          int    `json:"port,omitempty"`
	Username      string `json:"username,omitempty"`
	Insecure      bool   `json:"insecure,omitempty"`
	KeyName       string `json:"keyName,omitempty"`
	Password      string `json:"password,omitempty"`
	PrivateKey    string `json:"privateKey,omitempty"`
	KeyPassphrase string `json:"keyPassphrase,omitempty"`
}

// MergeMachine keeps a saved password or private key when the form leaves it blank.
// A new value replaces the saved one. Removing the key clears it.
func MergeMachine(prev Machine, in rpc.DAConn) Machine {
	out := Machine{
		Mode:     strings.TrimSpace(in.Mode),
		Host:     strings.TrimSpace(in.Host),
		Port:     in.Port,
		Username: strings.TrimSpace(in.Username),
		Insecure: in.Insecure,
		KeyName:  strings.TrimSpace(in.KeyName),
	}
	if in.Password != "" {
		out.Password = in.Password
	} else if in.UseSavedPassword {
		out.Password = prev.Password
	}
	ssh := out.Mode == "" || strings.EqualFold(out.Mode, "ssh")
	if !ssh {
		out.PrivateKey = prev.PrivateKey
		out.KeyPassphrase = prev.KeyPassphrase
		out.KeyName = prev.KeyName
	} else if strings.TrimSpace(in.PrivateKey) != "" {
		out.PrivateKey = in.PrivateKey
		if out.KeyName == "" {
			out.KeyName = prev.KeyName
		}
	} else if in.UseSavedKey {
		out.PrivateKey = prev.PrivateKey
		if out.KeyName == "" {
			out.KeyName = prev.KeyName
		}
	}
	if out.PrivateKey == "" {
		out.KeyName = ""
		out.KeyPassphrase = ""
	} else if in.KeyPassphrase != "" && ssh {
		out.KeyPassphrase = in.KeyPassphrase
	} else if in.UseSavedPassphrase {
		out.KeyPassphrase = prev.KeyPassphrase
	}
	return out
}

func LoadMachine(path string) (Machine, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Machine{}, nil
		}
		return Machine{}, err
	}
	var m Machine
	if err := json.Unmarshal(b, &m); err != nil {
		return Machine{}, err
	}
	return m, nil
}

func SaveMachine(path string, m Machine) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

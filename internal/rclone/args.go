package rclone

import (
	"fmt"
	"strings"
)

var remoteFields = map[string][]string{
	"s3":     {"provider", "access_key_id", "secret_access_key", "region", "endpoint"},
	"b2":     {"account", "key"},
	"sftp":   {"host", "user", "port", "pass"},
	"ftp":    {"host", "user", "port", "pass"},
	"webdav": {"url", "vendor", "user", "pass"},
	"local":  {},
}

var remoteRequired = map[string][]string{
	"s3":     {"access_key_id", "secret_access_key"},
	"b2":     {"account", "key"},
	"sftp":   {"host", "user"},
	"ftp":    {"host", "user"},
	"webdav": {"url", "vendor"},
}

var actions = map[string]struct {
	needsDest bool
	slow      bool
}{
	"copy":   {true, true},
	"sync":   {true, true},
	"move":   {true, true},
	"copyto": {true, true},
	"check":  {true, true},
	"ls":     {},
	"lsd":    {},
	"size":   {},
	"mkdir":  {},
	"delete": {slow: true},
	"purge":  {slow: true},
}

func validRemoteName(name string) error {
	if name == "" || len(name) > 32 {
		return fmt.Errorf("invalid remote name")
	}
	for i, c := range name {
		ok := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c == '-'
		if i > 0 && c >= '0' && c <= '9' {
			ok = true
		}
		if !ok {
			return fmt.Errorf("invalid remote name")
		}
	}
	return nil
}

func validValue(v string) error {
	if strings.ContainsAny(v, "\r\n\t") || strings.HasPrefix(v, "-") {
		return fmt.Errorf("invalid remote value")
	}
	if len(v) > 512 {
		return fmt.Errorf("remote value is too long")
	}
	return nil
}

func validTarget(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, "\r\n\t") {
		return fmt.Errorf("path is required")
	}
	if name, rest, ok := strings.Cut(raw, ":"); ok {
		if err := validRemoteName(name); err != nil {
			return fmt.Errorf("invalid remote in path")
		}
		if strings.Contains(rest, "..") {
			return fmt.Errorf("path must not contain ..")
		}
		return nil
	}
	if !strings.HasPrefix(raw, "/") || strings.Contains(raw, "..") {
		return fmt.Errorf("local path must be an absolute path")
	}
	return nil
}

// CreateArgs builds `rclone config create` arguments for a known backend.
func CreateArgs(name, typ string, params map[string]string) ([]string, error) {
	if err := validRemoteName(name); err != nil {
		return nil, err
	}
	fields, ok := remoteFields[typ]
	if !ok {
		return nil, fmt.Errorf("unsupported remote type")
	}
	allowed := map[string]struct{}{}
	for _, f := range fields {
		allowed[f] = struct{}{}
	}
	clean := map[string]string{}
	for k, v := range params {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := allowed[k]; !ok {
			return nil, fmt.Errorf("unsupported field %s", k)
		}
		if err := validValue(v); err != nil {
			return nil, err
		}
		clean[k] = v
	}
	if typ == "s3" && clean["provider"] == "" {
		clean["provider"] = "Other"
	}
	for _, key := range remoteRequired[typ] {
		if clean[key] == "" {
			return nil, fmt.Errorf("%s is required", strings.ReplaceAll(key, "_", " "))
		}
	}
	args := []string{"config", "create", name, typ}
	for _, key := range fields {
		if clean[key] == "" {
			continue
		}
		args = append(args, key, clean[key])
	}
	return args, nil
}

// RunArgs builds an rclone command for one supported action.
// The bool is true when the command may move a lot of data.
func RunArgs(action, source, dest string) ([]string, bool, error) {
	spec, ok := actions[action]
	if !ok {
		return nil, false, fmt.Errorf("unsupported action")
	}
	if err := validTarget(source); err != nil {
		return nil, false, err
	}
	args := []string{action, strings.TrimSpace(source)}
	if spec.needsDest {
		if err := validTarget(dest); err != nil {
			return nil, false, err
		}
		args = append(args, strings.TrimSpace(dest))
	}
	return args, spec.slow, nil
}

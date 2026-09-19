//go:build linux

package users

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/validate"
)

type Manager struct {
	HomeRoot string
}

func (m *Manager) Create(username, password string) (*rpc.UserResp, error) {
	if err := validate.LinuxUser(username); err != nil {
		return nil, err
	}
	if len(password) < 8 {
		return nil, fmt.Errorf("password must be at least 8 characters")
	}
	if _, err := user.Lookup(username); err == nil {
		return nil, fmt.Errorf("user already exists")
	}
	home := filepath.Join(m.HomeRoot, username)
	cmd := exec.Command("useradd", "-m", "-d", home, "-s", "/bin/bash", username)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("useradd: %s: %w", strings.TrimSpace(string(out)), err)
	}
	chpw := exec.Command("chpasswd")
	chpw.Stdin = strings.NewReader(username + ":" + password + "\n")
	if out, err := chpw.CombinedOutput(); err != nil {
		exec.Command("userdel", "-r", username).Run()
		return nil, fmt.Errorf("chpasswd: %s: %w", strings.TrimSpace(string(out)), err)
	}
	if err := os.MkdirAll(filepath.Join(home, "domains"), 0755); err != nil {
		return nil, err
	}
	for _, d := range []string{
		filepath.Join(home, "mail"),
		filepath.Join(home, "tmp"),
	} {
		if err := os.MkdirAll(d, 0750); err != nil {
			return nil, err
		}
	}
	if err := exec.Command("chown", "-R", username+":"+username, home).Run(); err != nil {
		return nil, fmt.Errorf("chown home: %w", err)
	}
	if err := os.Chmod(home, 0711); err != nil {
		return nil, err
	}
	return m.Info(username)
}

func (m *Manager) SetPassword(username, password string) error {
	if err := validate.LinuxUser(username); err != nil {
		return err
	}
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	if _, err := user.Lookup(username); err != nil {
		return fmt.Errorf("user not found")
	}
	chpw := exec.Command("chpasswd")
	chpw.Stdin = strings.NewReader(username + ":" + password + "\n")
	if out, err := chpw.CombinedOutput(); err != nil {
		return fmt.Errorf("chpasswd: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (m *Manager) SetSSH(username string, enabled bool) error {
	if err := validate.LinuxUser(username); err != nil {
		return err
	}
	shell := "/usr/sbin/nologin"
	if enabled {
		shell = "/bin/bash"
	}
	out, err := exec.Command("usermod", "-s", shell, username).CombinedOutput()
	if err != nil {
		return fmt.Errorf("usermod: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (m *Manager) CreateFTP(owner, name, password, homeRel string) (*rpc.FTPUserResp, error) {
	login, err := validate.FTPLogin(owner, name)
	if err != nil {
		return nil, err
	}
	if len(password) < 8 {
		return nil, fmt.Errorf("password must be at least 8 characters")
	}
	if _, err := user.Lookup(login); err == nil {
		return nil, fmt.Errorf("ftp user already exists")
	}
	ownerU, err := user.Lookup(owner)
	if err != nil {
		return nil, fmt.Errorf("owner account not found")
	}
	if homeRel == "" {
		homeRel = "domains"
	}
	homeRel = filepath.ToSlash(filepath.Clean(homeRel))
	homeRel = strings.TrimPrefix(homeRel, "/")
	if homeRel == ".." || strings.HasPrefix(homeRel, "../") || strings.Contains(homeRel, "/../") {
		return nil, fmt.Errorf("invalid home path")
	}
	home := filepath.Join(m.HomeRoot, owner, filepath.FromSlash(homeRel))
	root := filepath.Join(m.HomeRoot, owner)
	if home != root && !strings.HasPrefix(home, root+string(os.PathSeparator)) {
		return nil, fmt.Errorf("ftp home must stay inside the account")
	}
	if err := os.MkdirAll(home, 0750); err != nil {
		return nil, err
	}
	cmd := exec.Command("useradd", "-M", "-d", home, "-s", "/usr/sbin/nologin", "-g", ownerU.Gid, login)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("useradd: %s: %w", strings.TrimSpace(string(out)), err)
	}
	chpw := exec.Command("chpasswd")
	chpw.Stdin = strings.NewReader(login + ":" + password + "\n")
	if out, err := chpw.CombinedOutput(); err != nil {
		exec.Command("userdel", login).Run()
		return nil, fmt.Errorf("chpasswd: %s: %w", strings.TrimSpace(string(out)), err)
	}
	_ = exec.Command("chown", owner+":"+owner, home).Run()
	return &rpc.FTPUserResp{Login: login, Home: home}, nil
}

func (m *Manager) Info(username string) (*rpc.UserResp, error) {
	u, err := user.Lookup(username)
	if err != nil {
		return nil, err
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	return &rpc.UserResp{
		Username:  username,
		UID:       uid,
		GID:       gid,
		Home:      u.HomeDir,
		Suspended: locked(username),
	}, nil
}

func (m *Manager) Suspend(username string, suspend bool) error {
	if err := validate.LinuxUser(username); err != nil {
		return err
	}
	flag := "-L"
	if !suspend {
		flag = "-U"
	}
	out, err := exec.Command("usermod", flag, username).CombinedOutput()
	if err != nil {
		return fmt.Errorf("usermod: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (m *Manager) Delete(username string) error {
	return m.deleteUser(username, true)
}

func (m *Manager) DeleteKeepHome(username string) error {
	return m.deleteUser(username, false)
}

func (m *Manager) deleteUser(username string, removeHome bool) error {
	if err := validate.LinuxUser(username); err != nil {
		return err
	}
	args := []string{username}
	if removeHome {
		args = []string{"-r", username}
	}
	out, err := exec.Command("userdel", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("userdel: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func locked(username string) bool {
	f, err := os.Open("/etc/shadow")
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	prefix := username + ":"
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, prefix) {
			rest := strings.TrimPrefix(line, prefix)
			return strings.HasPrefix(rest, "!") || strings.HasPrefix(rest, "*")
		}
	}
	return false
}

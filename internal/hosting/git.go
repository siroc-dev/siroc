//go:build linux

package hosting

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/validate"
)

func (m *Manager) GitKey(username string) (*rpc.GitKeyResp, error) {
	if err := validate.LinuxUser(username); err != nil {
		return nil, err
	}
	pub, key, err := ensureUserSSHKey(username)
	if err != nil {
		return nil, err
	}
	fp := sshFingerprint(pub)
	return &rpc.GitKeyResp{OK: true, PublicKey: strings.TrimSpace(pub), KeyPath: key + ".pub", Fingerprint: fp}, nil
}

func (m *Manager) GitDeploy(req rpc.GitDeployReq) (*rpc.GitDeployResp, error) {
	if err := validate.LinuxUser(req.Username); err != nil {
		return nil, err
	}
	repo, err := validate.GitRepo(req.Repo)
	if err != nil {
		return nil, err
	}
	branch, err := validate.GitBranch(req.Branch)
	if err != nil {
		return nil, err
	}
	command, err := validate.GitCommand(req.Command)
	if err != nil {
		return nil, err
	}
	abs, err := validate.AccountPath(m.HomeRoot, req.Username, req.Path, "")
	if err != nil {
		return nil, err
	}
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("git is not installed on the server")
	}
	pub, key, err := ensureUserSSHKey(req.Username)
	if err != nil {
		return nil, err
	}
	_ = pub
	if err := os.MkdirAll(abs, 0755); err != nil {
		return nil, err
	}
	_ = exec.Command("chown", req.Username+":"+req.Username, abs).Run()
	sshCmd := fmt.Sprintf("ssh -i %s -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=%s",
		key, filepath.Join(filepath.Dir(key), "known_hosts"))
	var log bytes.Buffer
	gitDir := filepath.Join(abs, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		if err := runUser(req.Username, abs, sshCmd, 3*time.Minute, &log, "git", "remote", "set-url", "origin", repo); err != nil {
			if err := runUser(req.Username, abs, sshCmd, 3*time.Minute, &log, "git", "remote", "add", "origin", repo); err != nil {
				return gitFail(&log, err)
			}
		}
	} else {
		if err := runUser(req.Username, abs, sshCmd, 3*time.Minute, &log, "git", "init"); err != nil {
			return gitFail(&log, err)
		}
		_ = runUser(req.Username, abs, sshCmd, time.Minute, &log, "git", "remote", "remove", "origin")
		if err := runUser(req.Username, abs, sshCmd, 3*time.Minute, &log, "git", "remote", "add", "origin", repo); err != nil {
			return gitFail(&log, err)
		}
	}
	if err := runUser(req.Username, abs, sshCmd, 10*time.Minute, &log, "git", "fetch", "--force", "origin", branch); err != nil {
		return gitFail(&log, err)
	}
	if err := runUser(req.Username, abs, sshCmd, 3*time.Minute, &log, "git", "checkout", "-f", "-B", branch, "origin/"+branch); err != nil {
		if err := runUser(req.Username, abs, sshCmd, 3*time.Minute, &log, "git", "reset", "--hard", "origin/"+branch); err != nil {
			return gitFail(&log, err)
		}
	} else {
		_ = runUser(req.Username, abs, sshCmd, 3*time.Minute, &log, "git", "reset", "--hard", "origin/"+branch)
	}
	head := readGitCommit(req.Username, abs, sshCmd)
	if strings.TrimSpace(command) != "" {
		if err := runUser(req.Username, abs, sshCmd, 10*time.Minute, &log, "bash", "-lc", wrapNpmBins(command)); err != nil {
			resp, fail := gitFail(&log, err)
			if resp != nil {
				resp.Commit = head
			}
			return resp, fail
		}
	}
	return &rpc.GitDeployResp{OK: true, Log: trimLog(log.String()), Message: "Deployed " + branch, Commit: head}, nil
}

func (m *Manager) GitHead(username, rel string) (*rpc.GitCommit, error) {
	if err := validate.LinuxUser(username); err != nil {
		return nil, err
	}
	abs, err := validate.AccountPath(m.HomeRoot, username, rel, "")
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(abs, ".git")); err != nil {
		return &rpc.GitCommit{}, nil
	}
	if c := readGitCommit(username, abs, ""); c != nil {
		return c, nil
	}
	return &rpc.GitCommit{}, nil
}

func readGitCommit(username, dir, sshCmd string) *rpc.GitCommit {
	var buf bytes.Buffer
	if err := runUser(username, dir, sshCmd, 30*time.Second, &buf, "git", "log", "-1", "--format=%H%n%s%n%an%n%aI"); err != nil {
		return nil
	}
	c, ok := ParseGitLog(buf.String())
	if !ok {
		return nil
	}
	return &c
}

func gitFail(log *bytes.Buffer, err error) (*rpc.GitDeployResp, error) {
	msg := strings.TrimSpace(err.Error())
	out := trimLog(log.String())
	if out != "" {
		return &rpc.GitDeployResp{OK: false, Log: out, Message: msg}, fmt.Errorf("%s", tailErr(out, msg))
	}
	return nil, err
}

func tailErr(log, msg string) string {
	if msg != "" && !strings.Contains(log, msg) {
		return strings.TrimSpace(log + "\n" + msg)
	}
	if log != "" {
		return log
	}
	return msg
}

func trimLog(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 64<<10 {
		return s[len(s)-64<<10:]
	}
	return s
}

func ensureUserSSHKey(username string) (pub, priv string, err error) {
	u, err := user.Lookup(username)
	if err != nil {
		return "", "", fmt.Errorf("unknown linux user %s", username)
	}
	home := u.HomeDir
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return "", "", err
	}
	_ = os.Chown(sshDir, atoi(u.Uid), atoi(u.Gid))
	_ = os.Chmod(sshDir, 0700)
	for _, name := range []string{"id_ed25519", "id_rsa", "siroc_deploy"} {
		priv = filepath.Join(sshDir, name)
		pubFile := priv + ".pub"
		if b, e := os.ReadFile(pubFile); e == nil && strings.TrimSpace(string(b)) != "" {
			return string(b), priv, nil
		}
	}
	priv = filepath.Join(sshDir, "id_ed25519")
	if _, err := os.Stat(priv); err == nil {
		if b, e := os.ReadFile(priv + ".pub"); e == nil {
			return string(b), priv, nil
		}
	}
	cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-N", "", "-f", priv, "-C", "siroc-"+username)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("ssh-keygen: %s", strings.TrimSpace(string(out)))
	}
	_ = os.Chown(priv, atoi(u.Uid), atoi(u.Gid))
	_ = os.Chown(priv+".pub", atoi(u.Uid), atoi(u.Gid))
	_ = os.Chmod(priv, 0600)
	_ = os.Chmod(priv+".pub", 0644)
	b, err := os.ReadFile(priv + ".pub")
	if err != nil {
		return "", "", err
	}
	return string(b), priv, nil
}

func sshFingerprint(pub string) string {
	cmd := exec.Command("ssh-keygen", "-lf", "/dev/stdin")
	cmd.Stdin = strings.NewReader(pub)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func runUser(username, dir, sshCmd string, timeout time.Duration, log *bytes.Buffer, name string, args ...string) error {
	u, err := user.Lookup(username)
	if err != nil {
		return err
	}
	home := u.HomeDir
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = []string{
		"HOME=" + home,
		"USER=" + username,
		"LOGNAME=" + username,
		"PATH=/usr/local/bin:/usr/bin:/bin:" + filepath.Join(home, ".local/bin") + ":" + filepath.Join(home, ".composer/vendor/bin"),
		"GIT_SSH_COMMAND=" + sshCmd,
		"GIT_TERMINAL_PROMPT=0",
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{Uid: uint32(atoi(u.Uid)), Gid: uint32(atoi(u.Gid))},
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	fmt.Fprintf(log, "$ %s %s\n", name, strings.Join(args, " "))
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	select {
	case err := <-done:
		out := strings.TrimSpace(buf.String())
		if out != "" {
			log.WriteString(out + "\n")
		}
		if err != nil {
			if out != "" {
				return fmt.Errorf("%s", out)
			}
			return err
		}
		return nil
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		log.WriteString("timed out\n")
		return fmt.Errorf("command timed out")
	}
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

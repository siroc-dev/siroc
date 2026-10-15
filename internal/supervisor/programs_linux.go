//go:build linux

package supervisor

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func List() (*rpc.SupervisorStatus, error) {
	programs, err := readPrograms()
	if err != nil {
		return nil, err
	}
	if installed() {
		out, _ := exec.Command("supervisorctl", "status").CombinedOutput()
		programs = ApplyStatus(programs, string(out))
	} else {
		programs = ApplyStatus(programs, "")
	}
	return &rpc.SupervisorStatus{
		Installed: installed(),
		Active:    active(),
		Programs:  programs,
	}, nil
}

func Install() (*rpc.SupervisorStatus, error) {
	cmd := exec.Command("apt-get", "install", "-y", "supervisor")
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("install Supervisor: %s", tailText(string(out)))
	}
	_ = exec.Command("systemctl", "enable", "--now", unit()).Run()
	return List()
}

func Service(action string) (*rpc.SupervisorStatus, error) {
	switch action {
	case "start", "stop", "restart":
	default:
		return nil, fmt.Errorf("unknown Supervisor action")
	}
	if !installed() {
		return nil, fmt.Errorf("Supervisor is not installed")
	}
	out, err := exec.Command("systemctl", action, unit()).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("supervisor service: %s", tailText(string(out)))
	}
	return List()
}

func Reload() (*rpc.SupervisorStatus, error) {
	if err := reload(); err != nil {
		return nil, err
	}
	return List()
}

func Create(req rpc.SupervisorProgramReq) (*rpc.SupervisorStatus, error) {
	prog, err := Prepare(req, nil)
	if err != nil {
		return nil, err
	}
	programs, err := readPrograms()
	if err != nil {
		return nil, err
	}
	for _, item := range programs {
		if item.Name == prog.Name {
			return nil, fmt.Errorf("program %s already exists", prog.Name)
		}
	}
	path := filepath.Join(confDir, prog.Name+".conf")
	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("config file %s already exists", path)
	}
	body, _, err := RewriteConf("", "add", prog)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(confDir, 0755); err != nil {
		return nil, err
	}
	_ = os.MkdirAll(defaultLog, 0755)
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		return nil, err
	}
	if err := reload(); err != nil {
		return nil, err
	}
	return List()
}

func Update(name string, req rpc.SupervisorProgramReq) (*rpc.SupervisorStatus, error) {
	current, err := findProgram(name)
	if err != nil {
		return nil, err
	}
	prog, err := Prepare(req, current)
	if err != nil {
		return nil, err
	}
	if err := rewrite(current.File, "update", prog); err != nil {
		return nil, err
	}
	if err := reload(); err != nil {
		return nil, err
	}
	return List()
}

func Delete(name string) (*rpc.SupervisorStatus, error) {
	current, err := findProgram(name)
	if err != nil {
		return nil, err
	}
	if current.Site {
		return nil, fmt.Errorf("this program belongs to a website")
	}
	_ = runCtl("stop", name)
	if err := rewrite(current.File, "delete", *current); err != nil {
		return nil, err
	}
	if err := reload(); err != nil {
		return nil, err
	}
	return List()
}

func Ctl(name, action string) (*rpc.SupervisorStatus, error) {
	switch action {
	case "start", "stop", "restart":
	default:
		return nil, fmt.Errorf("unknown program action")
	}
	if !installed() {
		return nil, fmt.Errorf("Supervisor is not installed")
	}
	if err := runCtl(action, name); err != nil {
		return nil, err
	}
	return List()
}

func runCtl(action, name string) error {
	out, err := exec.Command("supervisorctl", action, name).CombinedOutput()
	if err != nil && strings.Contains(string(out), "no such process") {
		out, err = exec.Command("supervisorctl", action, name+":*").CombinedOutput()
	}
	if err != nil {
		return fmt.Errorf("supervisorctl %s: %s", action, tailText(string(out)))
	}
	return nil
}

func Log(name string) (string, error) {
	current, err := findProgram(name)
	if err != nil {
		return "", err
	}
	if !AllowedLog(current.StdoutLog) {
		return "", fmt.Errorf("this program has no readable log")
	}
	return tailFile(current.StdoutLog)
}

func readPrograms() ([]rpc.SupervisorProgram, error) {
	matches, err := filepath.Glob(filepath.Join(confDir, "*.conf"))
	if err != nil {
		return nil, err
	}
	var out []rpc.SupervisorProgram
	seen := map[string]bool{}
	for _, file := range matches {
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		for _, prog := range ProgramsIn(file, string(b)) {
			if seen[prog.Name] {
				continue
			}
			seen[prog.Name] = true
			out = append(out, prog)
		}
	}
	return out, nil
}

func findProgram(name string) (*rpc.SupervisorProgram, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "..") {
		return nil, fmt.Errorf("program %s was not found", name)
	}
	programs, err := readPrograms()
	if err != nil {
		return nil, err
	}
	for i := range programs {
		if programs[i].Name == name {
			return &programs[i], nil
		}
	}
	return nil, fmt.Errorf("program %s was not found", name)
}

func rewrite(file, op string, prog rpc.SupervisorProgram) error {
	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	next, remove, err := RewriteConf(string(b), op, prog)
	if err != nil {
		return err
	}
	if remove {
		return os.Remove(file)
	}
	return os.WriteFile(file, []byte(next), 0644)
}

func reload() error {
	if !installed() {
		return fmt.Errorf("Supervisor is not installed")
	}
	for _, arg := range []string{"reread", "update"} {
		out, err := exec.Command("supervisorctl", arg).CombinedOutput()
		if err != nil {
			return fmt.Errorf("supervisorctl %s: %s", arg, tailText(string(out)))
		}
	}
	return nil
}

func installed() bool {
	if _, err := exec.LookPath("supervisorctl"); err == nil {
		return true
	}
	_, err := exec.LookPath("supervisord")
	return err == nil
}

func active() bool {
	if exec.Command("systemctl", "is-active", "--quiet", "supervisor").Run() == nil {
		return true
	}
	return exec.Command("systemctl", "is-active", "--quiet", "supervisord").Run() == nil
}

func unit() string {
	if exec.Command("systemctl", "cat", "supervisor").Run() == nil {
		return "supervisor"
	}
	return "supervisord"
}

func tailFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("read log: %s", err.Error())
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("log is not a file")
	}
	const max = 64 * 1024
	start := int64(0)
	if info.Size() > max {
		start = info.Size() - max
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return "", err
	}
	b, err := io.ReadAll(io.LimitReader(f, max))
	if err != nil {
		return "", err
	}
	text := string(b)
	if start > 0 {
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			text = text[i+1:]
		}
	}
	return text, nil
}

func tailText(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 400 {
		s = s[len(s)-400:]
	}
	if s == "" {
		return "command failed"
	}
	return s
}

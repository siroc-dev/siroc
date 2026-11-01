//go:build linux

package hosting

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
	"github.com/siroc-dev/siroc/internal/validate"
)

func wpCronPath(user, domain string) string {
	return "/etc/cron.d/siroc-wp-" + user + "-" + slug(domain)
}

func wpCronLog(user, domain string) string {
	return filepath.Join("/home", user, "tmp", "wp-cron-"+domain+".log")
}

func fillWPCron(st *rpc.WPStatus, user, root, domain string) {
	if st == nil {
		return
	}
	config := ""
	if b, err := os.ReadFile(filepath.Join(root, "wp-config.php")); err == nil {
		config = string(b)
	}
	st.Cron, st.CronSystem = wpCronState(fileOK(wpCronPath(user, domain)), config)
}

func setWPCron(user, root, domain string, on bool) error {
	if err := validate.LinuxUser(user); err != nil {
		return err
	}
	domain, err := validate.ASCIIHost(domain)
	if err != nil {
		return err
	}
	root = filepath.Clean(root)
	if on && !fileOK(filepath.Join(root, "wp-load.php")) && !fileOK(filepath.Join(root, "wp-cron.php")) {
		return fmt.Errorf("WordPress was not found in %s", root)
	}
	if err := writeWPCronDisabled(user, root); err != nil {
		return err
	}
	path := wpCronPath(user, domain)
	if !on {
		_ = os.Remove(path)
		return nil
	}
	log := wpCronLog(user, domain)
	if err := os.MkdirAll(filepath.Dir(log), 0750); err != nil {
		return err
	}
	_ = exec.Command("chown", user+":"+user, filepath.Dir(log)).Run()
	cmd := "cd " + shellQuote(root) + " && " + wpExec(root) + " cron event run --due-now --path=" + shellQuote(root) + " --skip-themes"
	if url := wpURLArg(root); url != "" {
		cmd += " " + url
	}
	line := "# Siroc WordPress cron\n* * * * * " + user + " " + cmd + " >> " + shellQuote(log) + " 2>&1\n"
	if err := os.MkdirAll("/etc/cron.d", 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(line), 0644)
}

func writeWPCronDisabled(user, root string) error {
	path := filepath.Join(root, "wp-config.php")
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read wp-config.php: %w", err)
	}
	next := setWPCronDisabled(string(b), true)
	if next == string(b) {
		return nil
	}
	mode := os.FileMode(0640)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	if err := os.WriteFile(path, []byte(next), mode); err != nil {
		return err
	}
	_ = exec.Command("chown", user+":"+user, path).Run()
	return nil
}

func cleanupWPCron(user, domain string) {
	if user == "" || domain == "" {
		return
	}
	_ = os.Remove(wpCronPath(user, domain))
}

func renameWPCron(user, oldDomain, newDomain string) {
	b, err := os.ReadFile(wpCronPath(user, oldDomain))
	if err != nil {
		return
	}
	s := string(b)
	s = strings.ReplaceAll(s, "/domains/"+oldDomain+"/", "/domains/"+newDomain+"/")
	s = strings.ReplaceAll(s, "wp-cron-"+oldDomain+".log", "wp-cron-"+newDomain+".log")
	s = strings.ReplaceAll(s, oldDomain, newDomain)
	_ = os.WriteFile(wpCronPath(user, newDomain), []byte(s), 0644)
	_ = os.Remove(wpCronPath(user, oldDomain))
}

func moveWPCron(oldUser, newUser, domain, oldHome, newHome string) {
	if oldUser == newUser || oldUser == "" || newUser == "" {
		return
	}
	b, err := os.ReadFile(wpCronPath(oldUser, domain))
	if err != nil {
		return
	}
	s := string(b)
	if oldHome != "" && newHome != "" {
		s = strings.ReplaceAll(s, oldHome, newHome)
	}
	s = strings.ReplaceAll(s, " "+oldUser+" ", " "+newUser+" ")
	_ = os.WriteFile(wpCronPath(newUser, domain), []byte(s), 0644)
	_ = os.Remove(wpCronPath(oldUser, domain))
}

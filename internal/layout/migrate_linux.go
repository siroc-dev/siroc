//go:build linux

package layout

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// Migrate moves Siroc data under /siroc and leaves the old path as a symlink.
// Account homes stay in /home. It is safe to run on every start and during an update.
func Migrate() error {
	if err := os.MkdirAll(Root, 0755); err != nil {
		return err
	}
	var errs []string
	note := func(err error) {
		if err != nil {
			errs = append(errs, err.Error())
			log.Printf("siroc layout: %v", err)
		}
	}
	note(relocate("/var/lib/siroc", Data, true, nil))
	note(relocate("/var/log/nginx/sites", filepath.Join(Log, "nginx"), true, nil))
	note(relocate("/var/log/apache2/sites", filepath.Join(Log, "apache"), true, nil))
	note(relocate("/var/cache/nginx/siroc", Cache, true, nil))
	note(relocate("/var/lib/mysql", DB, false, []string{"mariadb", "mysql"}))
	note(relocate("/etc/nginx/sites-available", filepath.Join(Etc, "nginx", "sites-available"), true, nil))
	note(relocate("/etc/nginx/sites-enabled", filepath.Join(Etc, "nginx", "sites-enabled"), true, nil))
	note(relocate("/etc/nginx/conf.d", filepath.Join(Etc, "nginx", "conf.d"), true, nil))
	note(relocate("/etc/nginx/snippets", filepath.Join(Etc, "nginx", "snippets"), true, nil))
	note(relocate("/etc/apache2/sites-available", filepath.Join(Etc, "apache2", "sites-available"), true, nil))
	note(relocate("/etc/apache2/sites-enabled", filepath.Join(Etc, "apache2", "sites-enabled"), true, nil))
	note(relocate("/etc/mysql/conf.d", filepath.Join(Etc, "mysql", "conf.d"), false, nil))
	if _, err := os.Stat("/opt/siroc/go.mod"); err != nil {
		note(relocate("/opt/siroc/web", Web, false, nil))
	}
	note(linkBins())
	note(rewriteUnits())
	note(allowAppArmor())
	if len(errs) > 0 {
		return fmt.Errorf("%s", errs[0])
	}
	return nil
}

func relocate(src, dst string, create bool, stop []string) error {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)
	info, err := os.Lstat(src)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if !create {
			return nil
		}
		if _, err := os.Stat(filepath.Dir(src)); err != nil {
			return nil
		}
		if err := os.MkdirAll(dst, 0755); err != nil {
			return err
		}
		return os.Symlink(dst, src)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		if filepath.Clean(target) == dst {
			return nil
		}
		return fmt.Errorf("%s already points at %s", src, target)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", src)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	if dstInfo, err := os.Lstat(dst); err == nil {
		if dstInfo.Mode()&os.ModeSymlink != 0 || !dstInfo.IsDir() {
			return fmt.Errorf("%s is in the way", dst)
		}
		empty, err := dirEmpty(dst)
		if err != nil {
			return err
		}
		if !empty {
			return fmt.Errorf("%s already has files", dst)
		}
		if err := os.Remove(dst); err != nil {
			return err
		}
	}
	move := func() error {
		err := os.Rename(src, dst)
		if err == nil {
			return nil
		}
		if !errors.Is(err, syscall.EXDEV) {
			return err
		}
		stopped, err := stopServices(stop)
		if err != nil {
			return err
		}
		err = copyAcross(src, dst)
		for _, svc := range stopped {
			_ = exec.Command("systemctl", "start", svc).Run()
		}
		return err
	}
	if err := move(); err != nil {
		return err
	}
	if err := os.Symlink(dst, src); err != nil {
		return err
	}
	log.Printf("siroc layout: %s -> %s", src, dst)
	return nil
}

func copyAcross(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	cmd := exec.Command("cp", "-a", src+"/.", dst)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("copy %s: %s", src, strings.TrimSpace(string(out)))
	}
	if err := os.RemoveAll(src); err != nil {
		return err
	}
	return nil
}

func stopServices(services []string) ([]string, error) {
	var stopped []string
	for _, svc := range services {
		if exec.Command("systemctl", "is-active", "--quiet", svc).Run() != nil {
			continue
		}
		out, err := exec.Command("systemctl", "stop", svc).CombinedOutput()
		if err != nil {
			for _, started := range stopped {
				_ = exec.Command("systemctl", "start", started).Run()
			}
			return nil, fmt.Errorf("stop %s: %s", svc, strings.TrimSpace(string(out)))
		}
		stopped = append(stopped, svc)
	}
	return stopped, nil
}

func dirEmpty(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	_, err = f.Readdirnames(1)
	if err == io.EOF {
		return true, nil
	}
	return false, err
}

func linkBins() error {
	if err := os.MkdirAll(Bin, 0755); err != nil {
		return err
	}
	var errs []string
	for _, name := range []string{"siroc-agent", "siroc-panel", "siroc"} {
		if err := linkBin(name); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", errs[0])
	}
	return nil
}

func linkBin(name string) error {
	dst := filepath.Join(Bin, name)
	if _, err := os.Stat(dst); err != nil {
		for _, src := range []string{
			filepath.Join("/usr/local/bin", name),
			filepath.Join("/opt/siroc/bin", name),
		} {
			info, err := os.Lstat(src)
			if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				continue
			}
			if err := copyFile(src, dst, 0755); err != nil {
				return err
			}
			break
		}
	}
	if _, err := os.Stat(dst); err != nil {
		return nil
	}
	for _, link := range []string{
		filepath.Join("/usr/local/bin", name),
		filepath.Join("/opt/siroc/bin", name),
	} {
		if err := pointLink(link, dst); err != nil {
			return err
		}
	}
	return nil
}

func pointLink(link, dst string) error {
	if filepath.Clean(link) == filepath.Clean(dst) {
		return nil
	}
	parent := filepath.Dir(link)
	if _, err := os.Stat(parent); err != nil {
		return nil
	}
	info, err := os.Lstat(link)
	if err != nil {
		if os.IsNotExist(err) {
			return os.Symlink(dst, link)
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(link)
		if err != nil {
			return err
		}
		if filepath.Clean(target) == filepath.Clean(dst) {
			return nil
		}
	}
	if info.IsDir() {
		return nil
	}
	if err := os.Remove(link); err != nil {
		return err
	}
	return os.Symlink(dst, link)
}

func copyFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func rewriteUnits() error {
	matches, _ := filepath.Glob("/etc/systemd/system/siroc*")
	changed := false
	for _, path := range matches {
		info, err := os.Lstat(path)
		if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		next, ok := RewriteUnit(string(b))
		if !ok {
			continue
		}
		if err := os.WriteFile(path, []byte(next), info.Mode().Perm()); err != nil {
			return err
		}
		changed = true
		log.Printf("siroc layout: updated %s", path)
	}
	if !changed {
		return nil
	}
	out, err := exec.Command("systemctl", "daemon-reload").CombinedOutput()
	if err != nil {
		return fmt.Errorf("daemon-reload: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func allowAppArmor() error {
	if _, err := os.Stat("/etc/apparmor.d/local"); err != nil {
		return nil
	}
	body := "# Added by siroc so data under /siroc stays readable.\n" +
		"/siroc/data/ r,\n/siroc/data/** rw,\n" +
		"/siroc/log/ r,\n/siroc/log/** rw,\n" +
		"/siroc/cache/ r,\n/siroc/cache/** rw,\n" +
		"/siroc/db/ r,\n/siroc/db/** rwk,\n" +
		"/siroc/etc/ r,\n/siroc/etc/** rw,\n"
	for _, name := range []string{"usr.sbin.mysqld", "usr.sbin.mariadbd", "usr.sbin.nginx", "usr.sbin.apache2"} {
		if _, err := os.Stat("/etc/apparmor.d/" + name); err != nil {
			continue
		}
		path := "/etc/apparmor.d/local/" + name
		existing, _ := os.ReadFile(path)
		if !strings.Contains(string(existing), "/siroc/db/") {
			f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				return err
			}
			_, err = f.WriteString(body)
			closeErr := f.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
		}
		if out, err := exec.Command("apparmor_parser", "-r", "/etc/apparmor.d/"+name).CombinedOutput(); err != nil {
			log.Printf("siroc layout: apparmor %s: %s", name, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

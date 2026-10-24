//go:build linux

package software

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	nginxHeaderConf  = "/etc/nginx/conf.d/00-siroc-headers.conf"
	apacheHeaderPath = "/etc/apache2/conf-available/siroc-headers.conf"
)

// EnsureHeaderBuffers accepts a large Cookie header. Nginx otherwise answers
// 400 Request Header Or Cookie Too Large, and Apache would refuse the same
// request one hop later.
func EnsureHeaderBuffers() {
	ngx, saved, err := raiseNginxHeaderBuffers()
	if err != nil {
		log.Printf("request header buffers: %v", err)
		return
	}
	if ngx {
		if err := nginxConfigTest(); err != nil {
			for _, s := range saved {
				s.restore()
			}
			log.Printf("request header buffers: %v", err)
			return
		}
		if exec.Command("systemctl", "is-active", "--quiet", "nginx").Run() == nil {
			if out, err := exec.Command("systemctl", "reload", "nginx").CombinedOutput(); err != nil {
				log.Printf("request header buffers: reload nginx: %s", string(out))
			}
		}
	}
	if raiseApacheHeaderBuffers() && exec.Command("systemctl", "is-active", "--quiet", "apache2").Run() == nil {
		_ = exec.Command("systemctl", "reload", "apache2").Run()
	}
}

func raiseNginxHeaderBuffers() (bool, []savedFile, error) {
	if _, err := os.Stat("/etc/nginx"); err != nil {
		return false, nil, nil
	}
	if err := os.MkdirAll("/etc/nginx/conf.d", 0755); err != nil {
		return false, nil, err
	}
	var saved []savedFile
	changed := false
	if b, err := os.ReadFile(nginxHeaderConf); err != nil || string(b) != headerBufferConf {
		prev := remember(nginxHeaderConf)
		if err := os.WriteFile(nginxHeaderConf, []byte(headerBufferConf), 0644); err != nil {
			return false, nil, err
		}
		saved = append(saved, prev)
		changed = true
	}
	for _, path := range []string{"/etc/nginx/nginx.conf", "/etc/nginx/conf.d/99-cp-optimize.conf"} {
		ok, prev, err := dropHeaderBuffers(path)
		if err != nil {
			return changed, saved, err
		}
		if ok {
			saved = append(saved, prev)
			changed = true
		}
	}
	return changed, saved, nil
}

func dropHeaderBuffers(path string) (bool, savedFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return false, savedFile{}, nil
	}
	next, changed := withoutHeaderBuffers(string(b))
	if !changed {
		return false, savedFile{}, nil
	}
	prev := remember(path)
	return true, prev, os.WriteFile(path, []byte(next), prev.mode)
}

func raiseApacheHeaderBuffers() bool {
	if _, err := os.Stat("/etc/apache2/apache2.conf"); err != nil {
		return false
	}
	if err := os.MkdirAll(filepath.Dir(apacheHeaderPath), 0755); err != nil {
		return false
	}
	changed := false
	if b, err := os.ReadFile(apacheHeaderPath); err != nil || string(b) != apacheHeaderConf {
		if err := os.WriteFile(apacheHeaderPath, []byte(apacheHeaderConf), 0644); err != nil {
			log.Printf("request header buffers: %v", err)
			return false
		}
		changed = true
	}
	enabled := apacheHeaderEnabled()
	if exec.Command("a2enconf", "siroc-headers").Run() != nil {
		return changed
	}
	return changed || !enabled
}

type savedFile struct {
	path    string
	body    []byte
	mode    os.FileMode
	existed bool
}

func remember(path string) savedFile {
	s := savedFile{path: path, mode: 0644}
	b, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	s.existed = true
	s.body = b
	if info, err := os.Stat(path); err == nil {
		s.mode = info.Mode().Perm()
	}
	return s
}

func (s savedFile) restore() {
	if !s.existed {
		_ = os.Remove(s.path)
		return
	}
	_ = os.WriteFile(s.path, s.body, s.mode)
}

func apacheHeaderEnabled() bool {
	_, err := os.Stat("/etc/apache2/conf-enabled/siroc-headers.conf")
	return err == nil
}

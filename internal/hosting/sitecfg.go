//go:build linux

package hosting

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/siroc-dev/siroc/internal/validate"
)

func (m *Manager) ReadSiteConfig(domain string) (nginx, apache string, err error) {
	domain, err = validate.ASCIIHost(domain)
	if err != nil {
		return "", "", err
	}
	nginx, err = readConfFile(nginxSitePath(domain))
	if err != nil {
		return "", "", err
	}
	apache, err = readConfFile(apacheSitePath(domain))
	return nginx, apache, err
}

func (m *Manager) ApplySiteConfig(domain, nginxBody, apacheBody string, enabled, keep bool) (string, error) {
	domain, err := validate.ASCIIHost(domain)
	if err != nil {
		return "", err
	}
	if err := validate.SiteConfigText("nginx", nginxBody); err != nil {
		return "", err
	}
	if strings.TrimSpace(nginxBody) == "" {
		return "", fmt.Errorf("nginx config is empty")
	}
	if err := validate.SiteConfigText("apache", apacheBody); err != nil {
		return "", err
	}

	nginxPath := nginxSitePath(domain)
	nginxBak, err := snapshotFile(nginxPath)
	if err != nil {
		return "", err
	}
	if err := writeConfFile(nginxPath, nginxBody); err != nil {
		return "", err
	}
	var apacheBak *fileSnap
	if strings.TrimSpace(apacheBody) != "" {
		apacheBak, err = snapshotFile(apacheSitePath(domain))
		if err != nil {
			_ = nginxBak.restore()
			return "", err
		}
		if err := writeConfFile(apacheSitePath(domain), apacheBody); err != nil {
			_ = nginxBak.restore()
			return "", err
		}
	}

	link := filepath.Join("/etc/nginx/sites-enabled", domain+".conf")
	createdLink := false
	if _, statErr := os.Lstat(link); statErr != nil {
		_ = os.MkdirAll("/etc/nginx/sites-enabled", 0755)
		if err := os.Symlink(nginxPath, link); err == nil {
			createdLink = true
		}
	}
	createdApache := false
	if apacheBak != nil {
		en := filepath.Join("/etc/apache2/sites-enabled", domain+".conf")
		if _, statErr := os.Lstat(en); statErr != nil {
			if exec.Command("a2ensite", domain+".conf").Run() == nil {
				createdApache = true
			}
		}
	}

	var log strings.Builder
	nlog, nerr := runCombined("nginx", "-t")
	writeLog(&log, nlog)
	var aerr error
	if apacheBak != nil {
		alog, err := runCombined("apache2ctl", "configtest")
		writeLog(&log, alog)
		aerr = err
	}
	text := strings.TrimSpace(log.String())

	restore := func() {
		_ = nginxBak.restore()
		if apacheBak != nil {
			_ = apacheBak.restore()
		}
		if createdLink {
			_ = os.Remove(link)
		}
		if createdApache {
			_ = exec.Command("a2dissite", domain+".conf").Run()
		}
	}
	if nerr != nil || aerr != nil {
		restore()
		if text == "" {
			text = "config test failed"
		}
		return text, fmt.Errorf("%s", text)
	}
	if !keep {
		restore()
		return text, nil
	}
	if createdLink && !enabled {
		_ = os.Remove(link)
	}
	if createdApache && !enabled {
		_ = exec.Command("a2dissite", domain+".conf").Run()
	}
	if !enabled {
		return text, nil
	}
	if err := exec.Command("systemctl", "reload", "nginx").Run(); err != nil {
		restore()
		return text, fmt.Errorf("reload nginx: %w", err)
	}
	if apacheBak != nil {
		if err := exec.Command("systemctl", "reload", "apache2").Run(); err != nil {
			_ = nginxBak.restore()
			_ = apacheBak.restore()
			_ = exec.Command("systemctl", "reload", "nginx").Run()
			_ = exec.Command("systemctl", "reload", "apache2").Run()
			return text, fmt.Errorf("reload apache: %w", err)
		}
	}
	return text, nil
}

func nginxSitePath(domain string) string {
	return filepath.Join("/etc/nginx/sites-available", domain+".conf")
}

func apacheSitePath(domain string) string {
	return filepath.Join("/etc/apache2/sites-available", domain+".conf")
}

func readConfFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(b), nil
}

type fileSnap struct {
	path    string
	data    []byte
	missing bool
}

func snapshotFile(path string) (*fileSnap, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &fileSnap{path: path, missing: true}, nil
		}
		return nil, err
	}
	return &fileSnap{path: path, data: append([]byte(nil), b...)}, nil
}

func (s *fileSnap) restore() error {
	if s.missing {
		err := os.Remove(s.path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.WriteFile(s.path, s.data, 0644)
}

func runCombined(bin string, args ...string) (string, error) {
	out, err := exec.Command(bin, args...).CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil && text == "" {
		text = err.Error()
	}
	return text, err
}

func writeLog(b *strings.Builder, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteByte('\n')
	}
	b.WriteString(text)
}

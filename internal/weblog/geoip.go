//go:build linux

package weblog

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const geoipDir = "/var/lib/siroc/geoip"

func EnsureGeoIP() []string {
	_ = os.MkdirAll(geoipDir, 0755)
	_ = exec.Command("apt-get", "-y", "install", "libmaxminddb0").Run()
	city := filepath.Join(geoipDir, "city.mmdb")
	asn := filepath.Join(geoipDir, "asn.mmdb")
	_ = refreshGeoDB(city, "city")
	_ = refreshGeoDB(asn, "asn")
	var out []string
	for _, p := range []string{city, asn} {
		st, err := os.Stat(p)
		if err == nil && st.Size() > 1024 {
			out = append(out, p)
		}
	}
	return out
}

func refreshGeoDB(dest, kind string) error {
	if st, err := os.Stat(dest); err == nil && time.Since(st.ModTime()) < 40*24*time.Hour && st.Size() > 1024 {
		return nil
	}
	tmp := dest + ".gz"
	var last error
	for _, month := range geoipMonths(time.Now()) {
		url := dbipLiteURL(kind, month)
		cmd := exec.Command("curl", "-fsSL", "--max-time", "90", "-o", tmp, url)
		if out, err := cmd.CombinedOutput(); err != nil {
			last = fmt.Errorf("download %s: %s", kind, stringsTrim(out, err))
			continue
		}
		if err := gunzipFile(tmp, dest); err != nil {
			last = err
			_ = os.Remove(tmp)
			continue
		}
		_ = os.Remove(tmp)
		_ = os.Chmod(dest, 0644)
		return nil
	}
	_ = os.Remove(tmp)
	return last
}

func gunzipFile(src, dest string) error {
	cmd := exec.Command("gzip", "-dc", src)
	f, err := os.Create(dest + ".tmp")
	if err != nil {
		return err
	}
	cmd.Stdout = f
	err = cmd.Run()
	_ = f.Close()
	if err != nil {
		_ = os.Remove(dest + ".tmp")
		return err
	}
	return os.Rename(dest+".tmp", dest)
}

func stringsTrim(out []byte, err error) string {
	s := string(out)
	if s == "" && err != nil {
		return err.Error()
	}
	return s
}

//go:build linux

package software

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
)

const installLogDir = "/var/lib/siroc/install-logs"

var (
	installLogMu sync.Mutex
	installLogW  *os.File
)

func beginInstallLog(name, version string) func(error) {
	safe := installLogFile(name)
	if safe == "" {
		return func(error) {}
	}
	if err := os.MkdirAll(installLogDir, 0750); err != nil {
		return func(error) {}
	}
	f, err := os.OpenFile(filepath.Join(installLogDir, safe+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0640)
	if err != nil {
		return func(error) {}
	}
	fmt.Fprintf(f, "\n===== %s %s %s =====\n", time.Now().UTC().Format(time.RFC3339), name, strings.TrimSpace(version))
	_ = f.Sync()
	installLogMu.Lock()
	installLogW = f
	installLogMu.Unlock()
	return func(err error) {
		if err != nil {
			noteInstall("failed: " + err.Error())
		} else {
			noteInstall("done")
		}
		installLogMu.Lock()
		if installLogW == f {
			installLogW = nil
		}
		installLogMu.Unlock()
		_ = f.Close()
	}
}

func noteInstall(line string) {
	line = strings.TrimRight(line, "\r")
	if strings.TrimSpace(line) == "" {
		return
	}
	installLogMu.Lock()
	w := installLogW
	installLogMu.Unlock()
	if w == nil {
		return
	}
	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}
	_, _ = w.WriteString(line)
	_ = w.Sync()
}

func ReadInstallLog(name string, offset int64) (string, int64, error) {
	safe := installLogFile(name)
	if safe == "" {
		return "", 0, fmt.Errorf("invalid package")
	}
	f, err := os.Open(filepath.Join(installLogDir, safe+".log"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", 0, nil
		}
		return "", 0, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", 0, err
	}
	size := st.Size()
	if offset < 0 || offset > size {
		offset = 0
	}
	if offset == 0 && size > 128*1024 {
		offset = size - 128*1024
	}
	if _, err := f.Seek(offset, 0); err != nil {
		return "", offset, err
	}
	buf := make([]byte, 64*1024)
	var b strings.Builder
	remain := 256 * 1024
	for remain > 0 {
		nread := len(buf)
		if nread > remain {
			nread = remain
		}
		n, err := f.Read(buf[:nread])
		if n > 0 {
			b.Write(buf[:n])
			offset += int64(n)
			remain -= n
		}
		if err != nil {
			break
		}
	}
	return b.String(), offset, nil
}

func installLogFile(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || len(name) > 64 {
		return ""
	}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			continue
		}
		return ""
	}
	return name
}

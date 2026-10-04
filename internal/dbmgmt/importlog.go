//go:build linux

package dbmgmt

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/siroc-dev/siroc/internal/validate"
)

const importLogDir = "/var/lib/siroc/import-logs"

var (
	importLogMu sync.Mutex
	importLogs  = map[string]*os.File{}
)

func beginImportLog(name, filename string) func(error) {
	safe := importLogName(name)
	if safe == "" {
		return func(error) {}
	}
	if err := os.MkdirAll(importLogDir, 0750); err != nil {
		return func(error) {}
	}
	f, err := os.OpenFile(filepath.Join(importLogDir, safe+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0640)
	if err != nil {
		return func(error) {}
	}
	base := filepath.Base(strings.TrimSpace(filename))
	base = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, base)
	if len(base) > 180 {
		base = base[:180]
	}
	fmt.Fprintf(f, "\n===== %s %s %s =====\n", time.Now().UTC().Format(time.RFC3339), safe, base)
	_ = f.Sync()
	importLogMu.Lock()
	importLogs[safe] = f
	importLogMu.Unlock()
	return func(err error) {
		if err != nil {
			noteImport(safe, "failed: "+err.Error())
		} else {
			noteImport(safe, "done")
		}
		importLogMu.Lock()
		if importLogs[safe] == f {
			delete(importLogs, safe)
		}
		importLogMu.Unlock()
		_ = f.Close()
	}
}

func noteImport(name, line string) {
	line = strings.TrimRight(line, "\r")
	if strings.TrimSpace(line) == "" {
		return
	}
	safe := importLogName(name)
	if safe == "" {
		return
	}
	importLogMu.Lock()
	w := importLogs[safe]
	importLogMu.Unlock()
	if w == nil {
		return
	}
	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}
	_, _ = w.WriteString(line)
	_ = w.Sync()
}

func ReadImportLog(name string, offset int64) (string, int64, error) {
	safe := importLogName(name)
	if safe == "" {
		return "", 0, fmt.Errorf("invalid database")
	}
	f, err := os.Open(filepath.Join(importLogDir, safe+".log"))
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

func importLogName(name string) string {
	name = strings.TrimSpace(name)
	if validate.DBIdent(name) != nil {
		return ""
	}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			continue
		}
		return ""
	}
	return name
}

type byteNote struct {
	r     io.Reader
	name  string
	label string
	n     int64
	next  int64
	every int64
}

func (b *byteNote) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	if n > 0 {
		b.n += int64(n)
		if b.every > 0 && b.n >= b.next {
			b.next = b.n + b.every
			noteImport(b.name, b.label+" "+formatLogBytes(b.n))
		}
	}
	return n, err
}

func formatLogBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1fGB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

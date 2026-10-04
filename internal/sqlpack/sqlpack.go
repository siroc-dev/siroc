package sqlpack

import (
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	MaxUpload = 512 << 20
	MaxSQL    = 1 << 30
)

var skipLine = regexp.MustCompile(`(?i)^(CREATE\s+DATABASE|DROP\s+DATABASE|CREATE\s+USER|ALTER\s+USER|DROP\s+USER|RENAME\s+USER|GRANT\s+|REVOKE\s+|USE\s+)`)

func ExportMeta(db, format string) (filename, contentType string, err error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "sql":
		return db + ".sql", "application/sql", nil
	case "gz", "gzip":
		return db + ".sql.gz", "application/gzip", nil
	case "zip":
		return db + ".zip", "application/zip", nil
	default:
		return "", "", fmt.Errorf("format must be sql, gz, or zip")
	}
}

func Gzip(w io.Writer, r io.Reader) error {
	zw := gzip.NewWriter(w)
	if _, err := io.Copy(zw, r); err != nil {
		_ = zw.Close()
		return err
	}
	return zw.Close()
}

func Zip(w io.Writer, name string, r io.Reader) error {
	zw := zip.NewWriter(w)
	hdr := &zip.FileHeader{Name: name, Method: zip.Deflate}
	hdr.SetModTime(time.Now())
	entry, err := zw.CreateHeader(hdr)
	if err != nil {
		_ = zw.Close()
		return err
	}
	if _, err := io.Copy(entry, r); err != nil {
		_ = zw.Close()
		return err
	}
	return zw.Close()
}

// Open reads a .sql, .gz, or .zip upload and returns the SQL inside it.
func Open(filename string, r io.Reader) (io.ReadCloser, error) {
	kind, err := kindOf(filename)
	if err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp("", "siroc-import-*")
	if err != nil {
		return nil, err
	}
	n, copyErr := io.Copy(tmp, io.LimitReader(r, MaxUpload+1))
	if copyErr != nil || n > MaxUpload {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		if copyErr != nil {
			return nil, copyErr
		}
		return nil, fmt.Errorf("file is larger than 512MB")
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return nil, err
	}
	switch kind {
	case "sql":
		return &fileReader{f: tmp, r: limit(stripBOM(tmp))}, nil
	case "gz":
		gr, err := gzip.NewReader(tmp)
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
			return nil, fmt.Errorf("invalid gzip file")
		}
		return &gzipReader{f: tmp, z: gr, r: limit(stripBOM(gr))}, nil
	default:
		st, err := tmp.Stat()
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
			return nil, err
		}
		zr, err := zip.NewReader(tmp, st.Size())
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
			return nil, fmt.Errorf("invalid zip file")
		}
		var names []string
		for _, f := range zr.File {
			if f.FileInfo().IsDir() {
				continue
			}
			clean := path.Clean(strings.ReplaceAll(f.Name, "\\", "/"))
			if clean == "." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") || path.IsAbs(clean) {
				_ = tmp.Close()
				_ = os.Remove(tmp.Name())
				return nil, fmt.Errorf("zip entry %q is not allowed", f.Name)
			}
			if strings.HasSuffix(strings.ToLower(clean), ".sql") {
				names = append(names, f.Name)
			}
		}
		if len(names) == 0 {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
			return nil, fmt.Errorf("zip has no .sql file")
		}
		sort.Strings(names)
		return &zipReader{f: tmp, z: zr, names: names, left: MaxSQL}, nil
	}
}

func kindOf(filename string) (string, error) {
	n := strings.ToLower(path.Base(strings.ReplaceAll(filename, "\\", "/")))
	switch {
	case strings.HasSuffix(n, ".sql.gz"), strings.HasSuffix(n, ".gz"):
		return "gz", nil
	case strings.HasSuffix(n, ".zip"):
		return "zip", nil
	case strings.HasSuffix(n, ".sql"):
		return "sql", nil
	default:
		return "", fmt.Errorf("import a .sql, .gz, or .zip file")
	}
}

func limit(r io.Reader) io.Reader {
	return &limited{r: r, n: MaxSQL}
}

type limited struct {
	r io.Reader
	n int64
}

func (l *limited) Read(p []byte) (int, error) {
	if l.n <= 0 {
		return 0, fmt.Errorf("SQL dump is larger than 1GB")
	}
	if int64(len(p)) > l.n {
		p = p[:l.n]
	}
	n, err := l.r.Read(p)
	l.n -= int64(n)
	return n, err
}

func stripBOM(r io.Reader) io.Reader {
	br := bufio.NewReader(r)
	b, err := br.Peek(3)
	if err == nil && bytes.Equal(b, []byte{0xEF, 0xBB, 0xBF}) {
		_, _ = br.Discard(3)
	}
	return br
}

type fileReader struct {
	f *os.File
	r io.Reader
}

func (f *fileReader) Read(p []byte) (int, error) { return f.r.Read(p) }
func (f *fileReader) Close() error {
	err := f.f.Close()
	_ = os.Remove(f.f.Name())
	return err
}

type gzipReader struct {
	f *os.File
	z *gzip.Reader
	r io.Reader
}

func (g *gzipReader) Read(p []byte) (int, error) { return g.r.Read(p) }
func (g *gzipReader) Close() error {
	err := g.z.Close()
	_ = g.f.Close()
	_ = os.Remove(g.f.Name())
	return err
}

type zipReader struct {
	f     *os.File
	z     *zip.Reader
	names []string
	cur   io.ReadCloser
	sep   bool
	left  int64
}

func (z *zipReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		if z.sep {
			z.sep = false
			p[0] = '\n'
			return 1, nil
		}
		if z.cur == nil {
			if len(z.names) == 0 {
				return 0, io.EOF
			}
			name := z.names[0]
			z.names = z.names[1:]
			var entry *zip.File
			for _, f := range z.z.File {
				if f.Name == name {
					entry = f
					break
				}
			}
			if entry == nil {
				return 0, fmt.Errorf("missing zip entry %s", name)
			}
			rc, err := entry.Open()
			if err != nil {
				return 0, err
			}
			z.cur = rc
		}
		if z.left <= 0 {
			return 0, fmt.Errorf("SQL dump is larger than 1GB")
		}
		buf := p
		if int64(len(buf)) > z.left {
			buf = buf[:z.left]
		}
		n, err := z.cur.Read(buf)
		z.left -= int64(n)
		if n > 0 {
			if err != nil && err != io.EOF {
				return n, err
			}
			return n, nil
		}
		if err == io.EOF {
			_ = z.cur.Close()
			z.cur = nil
			if len(z.names) > 0 {
				z.sep = true
			}
			continue
		}
		if err != nil {
			return 0, err
		}
	}
}

func (z *zipReader) Close() error {
	if z.cur != nil {
		_ = z.cur.Close()
	}
	err := z.f.Close()
	_ = os.Remove(z.f.Name())
	return err
}

// Sanitize drops statements that would leave the selected database or change accounts.
func Sanitize(r io.Reader) io.Reader {
	return &sanitizer{r: bufio.NewReaderSize(r, 64*1024), atStart: true}
}

type sanitizer struct {
	r       *bufio.Reader
	pending []byte
	tailErr error
	atStart bool
	skip    bool
}

func (s *sanitizer) Read(p []byte) (int, error) {
	for {
		if len(s.pending) > 0 {
			n := copy(p, s.pending)
			s.pending = s.pending[n:]
			if len(s.pending) == 0 && s.tailErr != nil {
				err := s.tailErr
				s.tailErr = nil
				return n, err
			}
			return n, nil
		}
		chunk, err := s.r.ReadSlice('\n')
		line := append([]byte(nil), chunk...)
		end := err != bufio.ErrBufferFull
		if s.atStart && !s.skip && skipLine.Match(bytes.TrimSpace(trimNL(line))) {
			s.skip = true
		}
		if s.skip {
			if end {
				s.skip = false
				s.atStart = true
			}
			if err == bufio.ErrBufferFull {
				continue
			}
			if err != nil {
				return 0, err
			}
			continue
		}
		if end {
			s.atStart = true
		} else {
			s.atStart = false
		}
		if len(line) > 0 {
			s.pending = line
			if err == io.EOF {
				s.tailErr = io.EOF
			}
			continue
		}
		if err != nil {
			return 0, err
		}
	}
}

func trimNL(line []byte) []byte {
	return bytes.TrimRight(line, "\r\n")
}

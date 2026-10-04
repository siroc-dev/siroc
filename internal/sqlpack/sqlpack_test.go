package sqlpack

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io"
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	sql := "CREATE TABLE t (id INT);\nINSERT INTO t VALUES (1);\n"
	plain := mustOpen(t, "dump.sql", strings.NewReader(sql))
	if got := mustRead(t, plain); got != sql {
		t.Fatalf("sql %q", got)
	}

	var gz bytes.Buffer
	if err := Gzip(&gz, strings.NewReader(sql)); err != nil {
		t.Fatal(err)
	}
	got := mustRead(t, mustOpen(t, "dump.sql.gz", bytes.NewReader(gz.Bytes())))
	if got != sql {
		t.Fatalf("gz %q", got)
	}

	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	w, err := zw.Create("b.sql")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(w, "SELECT 2;\n")
	w, err = zw.Create("a.sql")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(w, "SELECT 1;\n")
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	got = mustRead(t, mustOpen(t, "dump.zip", bytes.NewReader(zbuf.Bytes())))
	if got != "SELECT 1;\n\nSELECT 2;\n" {
		t.Fatalf("zip %q", got)
	}
}

func TestRejectsBadArchive(t *testing.T) {
	if _, err := Open("notes.txt", strings.NewReader("x")); err == nil {
		t.Fatal("expected extension error")
	}
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	w, err := zw.Create("../evil.sql")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(w, "DROP DATABASE x;")
	_ = zw.Close()
	if _, err := Open("x.zip", bytes.NewReader(zbuf.Bytes())); err == nil {
		t.Fatal("expected zip slip error")
	}
	zbuf.Reset()
	zw = zip.NewWriter(&zbuf)
	w, _ = zw.Create("readme.txt")
	_, _ = io.WriteString(w, "hi")
	_ = zw.Close()
	if _, err := Open("x.zip", bytes.NewReader(zbuf.Bytes())); err == nil {
		t.Fatal("expected missing sql error")
	}
	var bad bytes.Buffer
	gw := gzip.NewWriter(&bad)
	_, _ = gw.Write([]byte("ok"))
	_ = gw.Close()
	bad.Truncate(2)
	if _, err := Open("x.gz", bytes.NewReader(bad.Bytes())); err == nil {
		t.Fatal("expected gzip error")
	}
}

func TestSanitize(t *testing.T) {
	in := "CREATE DATABASE `other`;\nUSE `other`;\nGRANT ALL ON *.* TO 'root'@'localhost';\nINSERT INTO t VALUES (1);\n" + strings.Repeat("A", 70*1024) + "\nDROP USER 'x'@'localhost';\nSELECT 1;\n"
	got := mustRead(t, io.NopCloser(Sanitize(strings.NewReader(in))))
	if strings.Contains(got, "CREATE DATABASE") || strings.Contains(got, "USE ") || strings.Contains(got, "GRANT ") || strings.Contains(got, "DROP USER") {
		t.Fatalf("kept admin statement: %s", got[:80])
	}
	if !strings.Contains(got, "INSERT INTO t") || !strings.Contains(got, "SELECT 1;") || !strings.Contains(got, strings.Repeat("A", 70*1024)) {
		t.Fatal("lost dump body")
	}
}

func TestExportMeta(t *testing.T) {
	name, typ, err := ExportMeta("app", "gz")
	if err != nil || name != "app.sql.gz" || typ != "application/gzip" {
		t.Fatalf("%s %s %v", name, typ, err)
	}
	if _, _, err := ExportMeta("app", "tar"); err == nil {
		t.Fatal("expected format error")
	}
}

func mustOpen(t *testing.T, name string, r io.Reader) io.ReadCloser {
	t.Helper()
	rc, err := Open(name, r)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rc.Close() })
	return rc
}

func mustRead(t *testing.T, r io.Reader) string {
	t.Helper()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

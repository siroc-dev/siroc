//go:build linux

package files

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func archiveKind(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.HasSuffix(n, ".tar.gz"), strings.HasSuffix(n, ".tgz"):
		return "targz"
	case strings.HasSuffix(n, ".tar.bz2"), strings.HasSuffix(n, ".tbz2"), strings.HasSuffix(n, ".tbz"):
		return "tarbz2"
	case strings.HasSuffix(n, ".tar.xz"), strings.HasSuffix(n, ".txz"):
		return "tarxz"
	case strings.HasSuffix(n, ".tar"):
		return "tar"
	case strings.HasSuffix(n, ".zip"):
		return "zip"
	case strings.HasSuffix(n, ".7z"):
		return "7z"
	case strings.HasSuffix(n, ".rar"):
		return "rar"
	case strings.HasSuffix(n, ".gz"):
		return "gz"
	case strings.HasSuffix(n, ".bz2"):
		return "bz2"
	case strings.HasSuffix(n, ".xz"):
		return "xz"
	default:
		return ""
	}
}

func stripArchiveName(name string) string {
	n := name
	lower := strings.ToLower(n)
	for _, s := range []string{".tar.gz", ".tar.bz2", ".tar.xz", ".tgz", ".tbz2", ".tbz", ".txz", ".tar", ".zip", ".7z", ".rar", ".gz", ".bz2", ".xz"} {
		if strings.HasSuffix(lower, s) {
			base := n[:len(n)-len(s)]
			if strings.TrimSpace(base) == "" {
				return name + "-extracted"
			}
			return base
		}
	}
	return name + "-extracted"
}

func uniquePath(path string) string {
	if _, err := os.Lstat(path); err != nil {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 2; i < 1000; i++ {
		cand := fmt.Sprintf("%s-%d%s", base, i, ext)
		if _, err := os.Lstat(cand); err != nil {
			return cand
		}
	}
	return path + "-extracted"
}

func (m *Manager) Extract(username, rel string, root bool) (string, error) {
	if archiveKind(rel) == "" {
		return "", fmt.Errorf("not a supported archive (zip, tar, tar.gz, tar.bz2, tar.xz, 7z, rar)")
	}
	if kind := archiveKind(rel); kind == "7z" || kind == "rar" || kind == "tarbz2" || kind == "tarxz" || kind == "bz2" || kind == "xz" {
		ensureUnpackers(kind)
	}
	if root {
		return m.rootExtract(rel)
	}
	abs, home, uid, gid, err := m.resolve(username, rel)
	if err != nil {
		return "", err
	}
	destAbs := uniquePath(filepath.Join(filepath.Dir(abs), stripArchiveName(filepath.Base(abs))))
	if err := stillJailed(home, destAbs); err != nil {
		return "", err
	}
	resp, err := m.call(uid, gid, helperReq{Op: "extract", Path: abs, Dest: destAbs, Home: home})
	if err != nil {
		return "", err
	}
	show, _ := filepath.Rel(home, destAbs)
	if resp != nil && resp.Dest != "" {
		if relShow, e := filepath.Rel(home, resp.Dest); e == nil {
			show = relShow
		}
	}
	return "/" + filepath.ToSlash(show), nil
}

func (m *Manager) Copy(username, rel, dest string, root bool) error {
	if root {
		return m.rootCopy(rel, dest)
	}
	src, home, uid, gid, err := m.resolve(username, rel)
	if err != nil {
		return err
	}
	dst, _, _, _, err := m.resolve(username, dest)
	if err != nil {
		return err
	}
	if src == dst {
		return fmt.Errorf("source and destination are the same")
	}
	_, err = m.call(uid, gid, helperReq{Op: "copy", Path: src, Dest: dst, Home: home})
	return err
}

func (m *Manager) ServeDownload(w http.ResponseWriter, username, rel string, root bool) error {
	var abs string
	var err error
	if root {
		abs, err = resolveRoot(rel)
	} else {
		abs, _, _, _, err = m.resolve(username, rel)
	}
	if err != nil {
		return err
	}
	st, err := os.Lstat(abs)
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("cannot download a symlink")
	}
	name := st.Name()
	if st.IsDir() {
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.tar.gz"`, strings.ReplaceAll(name, `"`, "")))
		gz := gzip.NewWriter(w)
		tw := tar.NewWriter(gz)
		_ = filepath.Walk(abs, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return nil
			}
			relWalk, err := filepath.Rel(abs, path)
			if err != nil || strings.HasPrefix(relWalk, "..") {
				return nil
			}
			hdr, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return nil
			}
			if relWalk == "." {
				return nil
			}
			hdr.Name = filepath.ToSlash(relWalk)
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			if info.Mode().IsRegular() {
				f, err := os.Open(path)
				if err != nil {
					return nil
				}
				_, _ = io.Copy(tw, io.LimitReader(f, 64<<20))
				_ = f.Close()
			}
			return nil
		})
		_ = tw.Close()
		_ = gz.Close()
		return nil
	}
	if st.Size() > 512<<20 {
		return fmt.Errorf("file too large to download (max 512MB)")
	}
	f, err := os.Open(abs)
	if err != nil {
		return err
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, strings.ReplaceAll(name, `"`, "")))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", st.Size()))
	_, err = io.Copy(w, f)
	return err
}

func ensureUnpackers(kind string) {
	need := []string{}
	switch kind {
	case "7z", "rar":
		if _, err := exec.LookPath("7z"); err != nil {
			if _, err2 := exec.LookPath("7za"); err2 != nil {
				need = append(need, "p7zip-full")
			}
		}
		if kind == "rar" {
			if _, err := exec.LookPath("unrar"); err != nil {
				if _, err2 := exec.LookPath("unar"); err2 != nil {
					need = append(need, "unrar")
				}
			}
		}
	case "tarbz2", "bz2":
		if _, err := exec.LookPath("bzip2"); err != nil {
			need = append(need, "bzip2")
		}
	case "tarxz", "xz":
		if _, err := exec.LookPath("xz"); err != nil {
			need = append(need, "xz-utils")
		}
	}
	if len(need) == 0 {
		return
	}
	args := append([]string{"install", "-y", "-qq"}, need...)
	_ = exec.Command("apt-get", args...).Run()
}

func extractArchive(src, dest string) error {
	if archiveKind(src) == "" {
		return fmt.Errorf("unsupported archive type")
	}
	if err := os.MkdirAll(dest, 0750); err != nil {
		return err
	}
	var err error
	switch archiveKind(src) {
	case "zip":
		err = unzipFile(src, dest)
	case "tar", "targz", "tarbz2", "tarxz":
		err = untarCmd(src, dest)
	case "gz":
		err = decompressOne(src, filepath.Join(dest, strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))), "gzip")
	case "bz2":
		err = cmdExtractFile("bzip2", "-dc", src, filepath.Join(dest, strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))))
	case "xz":
		err = cmdExtractFile("xz", "-dc", src, filepath.Join(dest, strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))))
	case "7z", "rar":
		err = sevenExtract(src, dest)
	default:
		err = fmt.Errorf("unsupported archive type")
	}
	if err != nil {
		_ = os.RemoveAll(dest)
		return err
	}
	if err := scrubExtract(dest); err != nil {
		_ = os.RemoveAll(dest)
		return err
	}
	return nil
}

func unzipFile(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return zipOpenError(src, err)
	}
	defer r.Close()
	for _, f := range r.File {
		out, err := safeJoin(dest, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(out, 0750); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(out), 0750); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		w, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
		if err != nil {
			_ = rc.Close()
			return err
		}
		_, err = io.Copy(w, io.LimitReader(rc, 256<<20))
		_ = w.Close()
		_ = rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func zipOpenError(src string, err error) error {
	head := make([]byte, 4)
	if f, e := os.Open(src); e == nil {
		_, _ = io.ReadFull(f, head)
		_ = f.Close()
	}
	if len(head) < 2 || head[0] != 'P' || head[1] != 'K' {
		return fmt.Errorf("not a zip file (wrong format or HTML download saved as .zip)")
	}
	st, _ := os.Stat(src)
	size := int64(0)
	if st != nil {
		size = st.Size()
	}
	msg := fmt.Sprintf("zip is incomplete or truncated (%s)", humanFileBytes(size))
	if size == 64<<20 {
		msg += "; the file was cut at 64MB during upload — upload or remote-download it again"
	}
	return fmt.Errorf("%s", msg)
}

func humanFileBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	u := []string{"B", "KB", "MB", "GB"}
	v := float64(n)
	i := 0
	for v >= 1024 && i < len(u)-1 {
		v /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", n)
	}
	if v >= 10 {
		return fmt.Sprintf("%.0f %s", v, u[i])
	}
	return fmt.Sprintf("%.1f %s", v, u[i])
}

func untarCmd(src, dest string) error {
	cmd := exec.Command("tar", "-xf", src, "-C", dest, "--no-absolute-names")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tar: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func decompressOne(src, dest, kind string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	var r io.Reader = f
	if kind == "gzip" {
		gr, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gr.Close()
		r = gr
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, io.LimitReader(r, 256<<20))
	return err
}

func cmdExtractFile(bin, flag, src, dest string) error {
	if _, err := exec.LookPath(bin); err != nil {
		return fmt.Errorf("%s is not installed", bin)
	}
	cmd := exec.Command(bin, flag, src)
	outf, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
	if err != nil {
		return err
	}
	defer outf.Close()
	cmd.Stdout = outf
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %s", bin, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func sevenExtract(src, dest string) error {
	bin := "7z"
	if _, err := exec.LookPath(bin); err != nil {
		if _, err2 := exec.LookPath("7za"); err2 == nil {
			bin = "7za"
		} else if archiveKind(src) == "rar" {
			if _, err3 := exec.LookPath("unrar"); err3 == nil {
				cmd := exec.Command("unrar", "x", "-o+", src, dest+string(os.PathSeparator))
				out, err := cmd.CombinedOutput()
				if err != nil {
					return fmt.Errorf("unrar: %s", strings.TrimSpace(string(out)))
				}
				return nil
			}
			return fmt.Errorf("install p7zip-full (or unrar) to extract this archive")
		} else {
			return fmt.Errorf("install p7zip-full to extract 7z archives")
		}
	}
	cmd := exec.Command(bin, "x", "-y", "-o"+dest, src)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("7z: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func safeJoin(dest, name string) (string, error) {
	name = strings.ReplaceAll(name, `\`, "/")
	cleaned := filepath.Join(dest, filepath.Clean(name))
	rel, err := filepath.Rel(dest, cleaned)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("archive contains an unsafe path")
	}
	return cleaned, nil
}

func scrubExtract(dest string) error {
	return filepath.Walk(dest, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, e := filepath.Rel(dest, path)
		if e != nil || strings.HasPrefix(rel, "..") {
			_ = os.RemoveAll(path)
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				_ = os.Remove(path)
				return nil
			}
			resolved := target
			if !filepath.IsAbs(target) {
				resolved = filepath.Join(filepath.Dir(path), target)
			}
			relT, err := filepath.Rel(dest, resolved)
			if err != nil || strings.HasPrefix(relT, "..") {
				_ = os.Remove(path)
			}
		}
		return nil
	})
}

func copyPath(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0750); err != nil {
		return err
	}
	dest = uniquePath(dest)
	cmd := exec.Command("cp", "-a", src, dest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("copy: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (m *Manager) rootExtract(rel string) (string, error) {
	abs, err := resolveRoot(rel)
	if err != nil {
		return "", err
	}
	destAbs := uniquePath(filepath.Join(filepath.Dir(abs), stripArchiveName(filepath.Base(abs))))
	if protectedRoot(destAbs) {
		return "", fmt.Errorf("cannot extract into a system path")
	}
	resp, err := runHelper(helperReq{Op: "extract", Path: abs, Dest: destAbs, Home: "/"})
	if err != nil {
		return "", err
	}
	if resp.Dest != "" {
		return filepath.ToSlash(resp.Dest), nil
	}
	return filepath.ToSlash(destAbs), nil
}

func (m *Manager) rootCopy(rel, dest string) error {
	src, err := resolveRoot(rel)
	if err != nil {
		return err
	}
	dst, err := resolveRoot(dest)
	if err != nil {
		return err
	}
	if protectedRoot(dst) {
		return fmt.Errorf("cannot copy onto a system path")
	}
	_, err = runHelper(helperReq{Op: "copy", Path: src, Dest: dst, Home: "/"})
	return err
}

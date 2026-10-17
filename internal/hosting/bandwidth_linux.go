//go:build linux

package hosting

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/siroc-dev/siroc/internal/rpc"
)

const bandwidthFormat = "log_format siroc_bw '$bytes_sent';\n"

func ensureBandwidthFormat() error {
	if err := os.MkdirAll("/etc/nginx/conf.d", 0755); err != nil {
		return err
	}
	return os.WriteFile("/etc/nginx/conf.d/00-siroc-bw.conf", []byte(bandwidthFormat), 0644)
}

var bandwidthMu sync.Mutex

type bandwidthState struct {
	Inode  uint64 `json:"inode"`
	Offset int64  `json:"offset"`
	Bytes  int64  `json:"bytes"`
}

// SiteBandwidth returns bytes nginx has sent for each website.
// The total is kept as the log grows and is not reset when the log rotates.
func SiteBandwidth(domains []string) ([]rpc.BandwidthSite, error) {
	if len(domains) > 500 {
		domains = domains[:500]
	}
	bandwidthMu.Lock()
	defer bandwidthMu.Unlock()
	if err := os.MkdirAll("/var/lib/siroc/bandwidth", 0750); err != nil {
		return nil, err
	}
	out := make([]rpc.BandwidthSite, 0, len(domains))
	seen := map[string]struct{}{}
	for _, domain := range domains {
		domain = strings.ToLower(strings.TrimSpace(domain))
		path, ok := bandwidthLogPath(domain)
		if !ok {
			continue
		}
		if _, ok := seen[domain]; ok {
			continue
		}
		seen[domain] = struct{}{}
		n, err := readBandwidth(domain, path)
		if err != nil {
			return nil, err
		}
		out = append(out, rpc.BandwidthSite{Domain: domain, Bytes: n})
	}
	return out, nil
}

func readBandwidth(domain, path string) (int64, error) {
	st := loadBandwidth(domain)
	info, err := os.Stat(path)
	if err != nil {
		return st.Bytes, nil
	}
	ino := inodeOf(info)
	if st.Inode != 0 && ino != 0 && ino != st.Inode {
		extra, _ := readInodeTail(filepath.Dir(path), st.Inode, st.Offset)
		st.Bytes += extra
		st.Offset = 0
	}
	if info.Size() < st.Offset {
		st.Offset = 0
	}
	f, err := os.Open(path)
	if err != nil {
		return st.Bytes, nil
	}
	defer f.Close()
	if _, err := f.Seek(st.Offset, io.SeekStart); err != nil {
		return st.Bytes, err
	}
	sum, consumed, err := SumLines(f)
	if err != nil {
		return st.Bytes, err
	}
	st.Bytes += sum
	st.Offset += consumed
	st.Inode = ino
	if err := saveBandwidth(domain, st); err != nil {
		return st.Bytes, err
	}
	return st.Bytes, nil
}

func readInodeTail(dir string, inode uint64, offset int64) (int64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, err := os.Stat(path)
		if err != nil || inodeOf(info) != inode {
			continue
		}
		if info.Size() <= offset {
			return 0, nil
		}
		f, err := os.Open(path)
		if err != nil {
			return 0, err
		}
		_, seekErr := f.Seek(offset, io.SeekStart)
		sum, _, readErr := SumLines(f)
		f.Close()
		if seekErr != nil {
			return 0, seekErr
		}
		return sum, readErr
	}
	return 0, nil
}

func bandwidthStatePath(domain string) string {
	return filepath.Join("/var/lib/siroc/bandwidth", domain+".json")
}

func loadBandwidth(domain string) bandwidthState {
	b, err := os.ReadFile(bandwidthStatePath(domain))
	if err != nil {
		return bandwidthState{}
	}
	var st bandwidthState
	if json.Unmarshal(b, &st) != nil || st.Bytes < 0 || st.Offset < 0 {
		return bandwidthState{}
	}
	return st
}

func saveBandwidth(domain string, st bandwidthState) error {
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return os.WriteFile(bandwidthStatePath(domain), b, 0640)
}

func inodeOf(info os.FileInfo) uint64 {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st == nil {
		return 0
	}
	return st.Ino
}

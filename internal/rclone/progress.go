package rclone

import (
	"encoding/json"
	"strings"

	"github.com/siroc-dev/siroc/internal/rpc"
)

type parsedProgress struct {
	Bytes          int64
	TotalBytes     int64
	Speed          float64
	Transfers      int64
	TotalTransfers int64
	Percent        int
	Files          []rpc.RcloneFileProgress
}

type statsJSON struct {
	Bytes          int64          `json:"bytes"`
	TotalBytes     int64          `json:"totalBytes"`
	Speed          float64        `json:"speed"`
	Transfers      int64          `json:"transfers"`
	TotalTransfers int64          `json:"totalTransfers"`
	Transferring   []transferJSON `json:"transferring"`
}

type transferJSON struct {
	Name       string  `json:"name"`
	Size       int64   `json:"size"`
	Bytes      int64   `json:"bytes"`
	Percentage int     `json:"percentage"`
	Speed      float64 `json:"speed"`
}

// Interpret reads one rclone output line.
// A stats line returns progress and an empty log so the live view is not flooded.
func Interpret(line string) (string, *parsedProgress) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", nil
	}
	if strings.HasPrefix(line, "{") {
		var raw struct {
			Level string     `json:"level"`
			Msg   string     `json:"msg"`
			Stats *statsJSON `json:"stats"`
		}
		if err := json.Unmarshal([]byte(line), &raw); err == nil && (raw.Level != "" || raw.Msg != "" || raw.Stats != nil) {
			if raw.Stats != nil {
				return "", progressFrom(raw.Stats)
			}
			return strings.TrimSpace(raw.Msg), nil
		}
	}
	return line, nil
}

func progressFrom(s *statsJSON) *parsedProgress {
	p := &parsedProgress{
		Bytes:          s.Bytes,
		TotalBytes:     s.TotalBytes,
		Speed:          s.Speed,
		Transfers:      s.Transfers,
		TotalTransfers: s.TotalTransfers,
		Percent:        percentOf(s.Bytes, s.TotalBytes),
	}
	if p.Percent == 0 {
		p.Percent = percentOf(s.Transfers, s.TotalTransfers)
	}
	for _, tr := range s.Transferring {
		name := strings.TrimSpace(tr.Name)
		if name == "" {
			continue
		}
		pct := tr.Percentage
		if tr.Size > 0 {
			pct = percentOf(tr.Bytes, tr.Size)
		}
		p.Files = append(p.Files, rpc.RcloneFileProgress{
			Name:    name,
			Bytes:   tr.Bytes,
			Size:    tr.Size,
			Percent: pct,
			Speed:   tr.Speed,
		})
	}
	return p
}

func percentOf(done, total int64) int {
	if total <= 0 || done <= 0 {
		return 0
	}
	if done >= total {
		return 100
	}
	return int(done * 100 / total)
}

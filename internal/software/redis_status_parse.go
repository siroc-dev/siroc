package software

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

var (
	redisIntRE   = regexp.MustCompile(`\(integer\)\s+(-?\d+)`)
	redisStrRE   = regexp.MustCompile(`"((?:\\.|[^"\\])*)"`)
	redisEntryRE = regexp.MustCompile(`^\s*\d+\)\s+1\)\s+\(integer\)`)
)

func parseRedisINFO(raw string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out
}

func parseRedisKeyspace(info map[string]string) int64 {
	var keys int64
	for k, v := range info {
		if !strings.HasPrefix(k, "db") {
			continue
		}
		for _, part := range strings.Split(v, ",") {
			name, val, ok := strings.Cut(part, "=")
			if ok && strings.TrimSpace(name) == "keys" {
				keys += parseInt64(val)
			}
		}
	}
	return keys
}

func parseRedisConfigValue(raw, key string) string {
	lines := []string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" {
			continue
		}
		if m := redisStrRE.FindStringSubmatch(line); m != nil {
			lines = append(lines, unquoteRedis(m[1]))
			continue
		}
		lines = append(lines, strings.Trim(line, `"`))
	}
	for i := 0; i < len(lines)-1; i++ {
		if strings.EqualFold(lines[i], key) {
			return lines[i+1]
		}
	}
	if len(lines) == 1 {
		return lines[0]
	}
	return ""
}

func parseRedisSlowlog(raw string) []rpc.RedisSlowLog {
	var out []rpc.RedisSlowLog
	var ints []int64
	var strs []string
	flush := func() {
		if len(ints) < 3 {
			ints, strs = nil, nil
			return
		}
		cmd, client := "", ""
		if len(strs) >= 2 {
			client = strs[len(strs)-2]
			cmd = strings.Join(strs[:len(strs)-2], " ")
		} else if len(strs) == 1 {
			cmd = strs[0]
		}
		if len(cmd) > 240 {
			cmd = cmd[:240] + "…"
		}
		item := rpc.RedisSlowLog{
			ID:         ints[0],
			DurationUs: ints[2],
			Command:    cmd,
			Client:     client,
		}
		if ints[1] > 0 {
			item.Time = time.Unix(ints[1], 0).UTC().Format("2006-01-02 15:04:05")
		}
		out = append(out, item)
		ints, strs = nil, nil
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if redisEntryRE.MatchString(line) && (len(ints) > 0 || len(strs) > 0) {
			flush()
		}
		if m := redisIntRE.FindStringSubmatch(line); m != nil && !strings.Contains(line, `"`) {
			ints = append(ints, parseInt64(m[1]))
			continue
		}
		for _, m := range redisStrRE.FindAllStringSubmatch(line, -1) {
			strs = append(strs, unquoteRedis(m[1]))
		}
	}
	flush()
	if len(out) > 20 {
		out = out[:20]
	}
	return out
}

func redisHitRate(hits, misses int64) float64 {
	total := hits + misses
	if total <= 0 {
		return 0
	}
	return float64(hits) * 100 / float64(total)
}

func unquoteRedis(s string) string {
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\\`, `\`)
	return s
}

func validateRedisConf(conf string) error {
	if strings.TrimSpace(conf) == "" {
		return fmt.Errorf("redis.conf is empty")
	}
	if len(conf) > 512<<10 {
		return fmt.Errorf("redis.conf is too large (max 512KB)")
	}
	return nil
}

func redisPublicBind(bind string) bool {
	b := strings.TrimSpace(bind)
	return b != "" && b != "127.0.0.1" && b != "127.0.0.1 -::1" && b != "::1"
}

func redisConfHasPassword(conf string) bool {
	for _, line := range strings.Split(conf, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "requirepass ") && !strings.HasPrefix(t, "requirepass \"\"") {
			return true
		}
	}
	return false
}

func parseInt64(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}

func parseFloat64(s string) float64 {
	n, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return n
}

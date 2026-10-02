//go:build linux

package software

import (
	"strings"
	"time"

	"github.com/siroc-dev/siroc/internal/rpc"
)

func RedisStatus() *rpc.RedisStatus {
	st := &rpc.RedisStatus{}
	ok, ver := redisInstalled()
	st.Installed = ok
	st.Version = strings.TrimSpace(ver)
	if !ok {
		st.Message = "Redis is not installed. Install it from Software."
		return st
	}
	st.Active = serviceActive("redis-server") || serviceActive("redis")
	if !st.Active {
		st.Message = "Redis is installed but not running."
		return st
	}
	infoRaw, err := redisCLI("INFO")
	if err != nil {
		st.Message = err.Error()
		return st
	}
	info := parseRedisINFO(infoRaw)
	if v := info["redis_version"]; v != "" {
		st.Version = v
	}
	st.UptimeSec = parseInt64(info["uptime_in_seconds"])
	st.ConnectedClients = int(parseInt64(info["connected_clients"]))
	st.BlockedClients = int(parseInt64(info["blocked_clients"]))
	st.UsedMemory = parseInt64(info["used_memory"])
	st.UsedMemoryPeak = parseInt64(info["used_memory_peak"])
	st.UsedMemoryHuman = info["used_memory_human"]
	st.MaxMemory = parseInt64(info["maxmemory"])
	st.OpsPerSec = parseInt64(info["instantaneous_ops_per_sec"])
	st.Hits = parseInt64(info["keyspace_hits"])
	st.Misses = parseInt64(info["keyspace_misses"])
	st.HitRate = redisHitRate(st.Hits, st.Misses)
	st.Keys = parseRedisKeyspace(info)
	st.EvictedKeys = parseInt64(info["evicted_keys"])
	st.ExpiredKeys = parseInt64(info["expired_keys"])
	st.Role = info["role"]
	if raw, err := redisCLI("CONFIG", "GET", "slowlog-log-slower-than"); err == nil {
		st.SlowLogSlowerThan = parseInt64(parseRedisConfigValue(raw, "slowlog-log-slower-than"))
	}
	if raw, err := redisCLI("SLOWLOG", "LEN"); err == nil {
		st.SlowLogLen = parseInt64(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "(integer) ")))
	}
	if raw, err := redisCLI("SLOWLOG", "GET", "20"); err == nil {
		st.SlowLog = parseRedisSlowlog(raw)
		if st.SlowLogLen == 0 {
			st.SlowLogLen = int64(len(st.SlowLog))
		}
	}
	st.Ready = true
	st.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	return st
}

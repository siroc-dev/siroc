package software

import "testing"

func TestParseRedisINFO(t *testing.T) {
	raw := `# Server
redis_version:7.2.4
uptime_in_seconds:3600
# Clients
connected_clients:4
# Memory
used_memory:1048576
# Stats
keyspace_hits:90
keyspace_misses:10
# Keyspace
db0:keys=5,expires=1,avg_ttl=0
db1:keys=2,expires=0,avg_ttl=0
`
	info := parseRedisINFO(raw)
	if info["redis_version"] != "7.2.4" || info["connected_clients"] != "4" {
		t.Fatalf("%v", info)
	}
	if parseRedisKeyspace(info) != 7 {
		t.Fatalf("keys=%d", parseRedisKeyspace(info))
	}
	if got := redisHitRate(90, 10); got < 89.9 || got > 90.1 {
		t.Fatalf("hit=%.2f", got)
	}
}

func TestParseRedisSlowlog(t *testing.T) {
	raw := `1) 1) (integer) 12
   2) (integer) 1700000000
   3) (integer) 15000
   4) 1) "KEYS"
      2) "*"
   5) "127.0.0.1:52344"
   6) ""
2) 1) (integer) 11
   2) (integer) 1699990000
   3) (integer) 2500
   4) 1) "GET"
      2) "user:1"
   5) "127.0.0.1:4000"
   6) "panel"
`
	got := parseRedisSlowlog(raw)
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].ID != 12 || got[0].DurationUs != 15000 || got[0].Command != "KEYS *" || got[0].Client != "127.0.0.1:52344" {
		t.Fatalf("%#v", got[0])
	}
	if got[1].Command != "GET user:1" || got[1].Client != "127.0.0.1:4000" {
		t.Fatalf("%#v", got[1])
	}
}

func TestValidateRedisConf(t *testing.T) {
	if err := validateRedisConf(""); err == nil {
		t.Fatal("empty")
	}
	if err := validateRedisConf("port 6379\n"); err != nil {
		t.Fatal(err)
	}
	if !redisPublicBind("0.0.0.0") || redisPublicBind("127.0.0.1") {
		t.Fatal("bind")
	}
	if !redisConfHasPassword("requirepass secret\n") || redisConfHasPassword("# requirepass x\n") {
		t.Fatal("password")
	}
}

func TestParseRedisConfigValue(t *testing.T) {
	raw := `1) "slowlog-log-slower-than"
2) "10000"
`
	if parseRedisConfigValue(raw, "slowlog-log-slower-than") != "10000" {
		t.Fatal(parseRedisConfigValue(raw, "slowlog-log-slower-than"))
	}
}

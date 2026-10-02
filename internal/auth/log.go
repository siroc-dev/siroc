package auth

import (
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const AuthLogPath = "/var/log/siroc/auth.log"

func ClientIP(r *http.Request) string {
	host := strings.TrimSpace(r.RemoteAddr)
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

func LogFailure(ip, username string) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return
	}
	username = strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\n' {
			return '_'
		}
		return r
	}, strings.TrimSpace(username))
	if username == "" {
		username = "-"
	}
	if len(username) > 64 {
		username = username[:64]
	}
	line := time.Now().UTC().Format("2006-01-02 15:04:05") + " siroc login failed ip=" + ip + " user=" + username + "\n"
	f, err := os.OpenFile(AuthLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		return
	}
	_, _ = f.WriteString(line)
	_ = f.Close()
}

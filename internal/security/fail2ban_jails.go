package security

import (
	"fmt"
	"strings"
)

// JailSpec is one Fail2ban jail written into jail.d.
type JailSpec struct {
	Name     string
	Filter   string
	Logs     []string
	Port     string
	MaxRetry int
	FindTime string
	BanTime  string
}

func RenderFail2banJails(specs []JailSpec) string {
	var b strings.Builder
	for _, s := range specs {
		if s.Name == "" || s.Filter == "" || len(s.Logs) == 0 {
			continue
		}
		if s.MaxRetry == 0 {
			s.MaxRetry = 5
		}
		if s.FindTime == "" {
			s.FindTime = "10m"
		}
		if s.BanTime == "" {
			s.BanTime = "1h"
		}
		if s.Port == "" {
			s.Port = "0:65535"
		}
		fmt.Fprintf(&b, "[%s]\nenabled = true\nfilter = %s\nbackend = polling\nport = %s\nmaxretry = %d\nfindtime = %s\nbantime = %s\n",
			s.Name, s.Filter, s.Port, s.MaxRetry, s.FindTime, s.BanTime)
		b.WriteString("logpath = " + s.Logs[0] + "\n")
		for _, p := range s.Logs[1:] {
			if strings.TrimSpace(p) != "" {
				b.WriteString("          " + p + "\n")
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

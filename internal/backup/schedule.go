package backup

import (
	"fmt"
	"sort"
	"strings"
)

// CronTask is one backup schedule written to /etc/cron.d/siroc-backup.
type CronTask struct {
	ID       int64
	Cycle    string
	Minute   int
	Hour     int
	Weekday  int
	Monthday int
	Enabled  bool
}

func cronSpec(t CronTask) (string, error) {
	if t.Minute < 0 || t.Minute > 59 {
		return "", fmt.Errorf("minute must be 0-59")
	}
	if t.Hour < 0 || t.Hour > 23 {
		return "", fmt.Errorf("hour must be 0-23")
	}
	switch t.Cycle {
	case "hourly":
		return fmt.Sprintf("%d * * * *", t.Minute), nil
	case "daily":
		return fmt.Sprintf("%d %d * * *", t.Minute, t.Hour), nil
	case "weekly":
		if t.Weekday < 0 || t.Weekday > 6 {
			return "", fmt.Errorf("weekday must be 0-6")
		}
		return fmt.Sprintf("%d %d * * %d", t.Minute, t.Hour, t.Weekday), nil
	case "monthly":
		if t.Monthday < 1 || t.Monthday > 28 {
			return "", fmt.Errorf("day of month must be 1-28")
		}
		return fmt.Sprintf("%d %d %d * *", t.Minute, t.Hour, t.Monthday), nil
	default:
		return "", fmt.Errorf("cycle must be hourly, daily, weekly, or monthly")
	}
}

func cleanBin(bin string) (string, error) {
	bin = strings.TrimSpace(bin)
	if bin == "" || !strings.HasPrefix(bin, "/") || strings.ContainsAny(bin, "\r\n\t '\"`$\\;") {
		return "", fmt.Errorf("panel binary path is not usable in cron")
	}
	base := bin
	if i := strings.LastIndex(bin, "/"); i >= 0 {
		base = bin[i+1:]
	}
	if base != "siroc-panel" {
		return "", fmt.Errorf("panel binary path is not usable in cron")
	}
	return bin, nil
}

// RenderCronFile builds /etc/cron.d/siroc-backup. Disabled tasks are omitted.
func RenderCronFile(bin string, tasks []CronTask) (string, error) {
	bin, err := cleanBin(bin)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("# Siroc backup schedules. Written by the panel.\n")
	b.WriteString("SHELL=/bin/sh\n")
	b.WriteString("PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\n")
	for _, t := range tasks {
		if !t.Enabled || t.ID <= 0 {
			continue
		}
		spec, err := cronSpec(t)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "%s root %s backup-cron %d >> /var/log/siroc/backup-cron.log 2>&1\n", spec, bin, t.ID)
	}
	body := b.String()
	if err := ValidCronFile(body); err != nil {
		return "", err
	}
	return body, nil
}

// ValidCronFile rejects anything that is not a Siroc backup cron file.
func ValidCronFile(body string) error {
	if strings.Contains(body, "\r") || !strings.HasSuffix(body, "\n") {
		return fmt.Errorf("invalid backup cron file")
	}
	for _, line := range strings.Split(strings.TrimSuffix(body, "\n"), "\n") {
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "SHELL=") || strings.HasPrefix(line, "PATH=") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 12 {
			return fmt.Errorf("invalid backup cron line")
		}
		for i := 0; i < 5; i++ {
			if fields[i] != "*" && !cronNum(fields[i]) {
				return fmt.Errorf("invalid backup cron line")
			}
		}
		if fields[5] != "root" || !strings.HasPrefix(fields[6], "/") || fields[7] != "backup-cron" || !cronNum(fields[8]) {
			return fmt.Errorf("invalid backup cron line")
		}
		if fields[9] != ">>" || fields[10] != "/var/log/siroc/backup-cron.log" || fields[11] != "2>&1" {
			return fmt.Errorf("invalid backup cron line")
		}
	}
	return nil
}

func cronNum(s string) bool {
	if s == "" || len(s) > 6 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Describe is the schedule shown in the panel.
func Describe(t CronTask) string {
	hm := fmt.Sprintf("%02d:%02d", t.Hour, t.Minute)
	switch t.Cycle {
	case "hourly":
		return fmt.Sprintf("Hourly at :%02d", t.Minute)
	case "weekly":
		days := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
		day := "Sunday"
		if t.Weekday >= 0 && t.Weekday < len(days) {
			day = days[t.Weekday]
		}
		return fmt.Sprintf("Weekly on %s at %s", day, hm)
	case "monthly":
		return fmt.Sprintf("Monthly on day %d at %s", t.Monthday, hm)
	default:
		return fmt.Sprintf("Daily at %s", hm)
	}
}

// PruneNames returns the oldest names to delete so keep remain. Names sort oldest first.
func PruneNames(names []string, keep int) []string {
	if keep < 1 || len(names) <= keep {
		return nil
	}
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	return sorted[:len(sorted)-keep]
}

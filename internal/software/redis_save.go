package software

import "strings"

// redisSaves reports whether an RDB snapshot is configured.
// No save lines means Redis keeps its built-in snapshot defaults.
func redisSaves(conf string) bool {
	saw := false
	on := false
	for _, line := range strings.Split(conf, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		key, val, ok := strings.Cut(t, " ")
		if !ok || key != "save" {
			continue
		}
		saw = true
		val = strings.Trim(strings.TrimSpace(val), `"`)
		on = val != ""
	}
	if !saw {
		return true
	}
	return on
}

func applyRedisPersistence(conf string, saveToDisk, appendOnly bool) string {
	if !saveToDisk {
		conf = commentRedisKey(conf, "save")
		conf = setRedisDirective(conf, "appendonly", "no")
		if !hasDirective(conf, `save ""`) {
			conf = appendDirective(conf, `save ""`)
		}
		return conf
	}
	conf = commentSaveEmpty(conf)
	if !hasSaveInterval(conf) {
		conf = appendDirective(conf, "save 900 1")
		conf = appendDirective(conf, "save 300 10")
		conf = appendDirective(conf, "save 60 10000")
	}
	if appendOnly {
		conf = setRedisDirective(conf, "appendonly", "yes")
	} else {
		conf = setRedisDirective(conf, "appendonly", "no")
	}
	return conf
}

func commentRedisKey(conf, key string) string {
	lines := strings.Split(conf, "\n")
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		field, _, _ := strings.Cut(t, " ")
		if field == key {
			lines[i] = "# " + t
		}
	}
	return strings.Join(lines, "\n")
}

func commentSaveEmpty(conf string) string {
	lines := strings.Split(conf, "\n")
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if t == `save ""` || t == "save" {
			lines[i] = "# " + t
		}
	}
	return strings.Join(lines, "\n")
}

func hasSaveInterval(conf string) bool {
	for _, line := range strings.Split(conf, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		key, val, ok := strings.Cut(t, " ")
		if !ok || key != "save" {
			continue
		}
		val = strings.Trim(strings.TrimSpace(val), `"`)
		if val != "" {
			return true
		}
	}
	return false
}

func hasDirective(conf, want string) bool {
	for _, line := range strings.Split(conf, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if t == want {
			return true
		}
	}
	return false
}

func appendDirective(conf, line string) string {
	if conf != "" && !strings.HasSuffix(conf, "\n") {
		conf += "\n"
	}
	return conf + line + "\n"
}

func setRedisDirective(conf, key, val string) string {
	lines := strings.Split(conf, "\n")
	found := false
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "#") {
			continue
		}
		field, _, _ := strings.Cut(t, " ")
		if field != key {
			continue
		}
		if !found {
			lines[i] = key + " " + val
			found = true
		} else {
			lines[i] = "# " + t
		}
	}
	if !found {
		return appendDirective(conf, key+" "+val)
	}
	return strings.Join(lines, "\n")
}

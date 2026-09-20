package software

import "strings"

var ubuntuVendorSuites = []string{"noble", "jammy", "focal"}
var debianVendorSuites = []string{"bookworm", "bullseye"}

func fallbackSuites(id, code string) []string {
	code = strings.TrimSpace(code)
	out := []string{}
	if code != "" {
		out = append(out, code)
	}
	list := ubuntuVendorSuites
	if id == "debian" {
		list = debianVendorSuites
	}
	for _, s := range list {
		if s != code {
			out = append(out, s)
		}
	}
	return out
}

func parseDebLine(line string) (prefix, url, suite, rest string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return
	}
	kind := ""
	switch {
	case strings.HasPrefix(line, "deb-src "):
		kind = "deb-src"
		line = strings.TrimSpace(line[8:])
	case strings.HasPrefix(line, "deb "):
		kind = "deb"
		line = strings.TrimSpace(line[4:])
	default:
		return
	}
	opts := ""
	if strings.HasPrefix(line, "[") {
		end := strings.Index(line, "]")
		if end < 0 {
			return
		}
		opts = line[:end+1]
		line = strings.TrimSpace(line[end+1:])
	}
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return
	}
	prefix = kind
	if opts != "" {
		prefix += " " + opts
	}
	url = fields[0]
	suite = fields[1]
	rest = strings.Join(fields[2:], " ")
	ok = true
	return
}

func replaceDebSuite(line, suite string) string {
	prefix, url, _, rest, ok := parseDebLine(line)
	if !ok {
		return line
	}
	if rest == "" {
		return prefix + " " + url + " " + suite
	}
	return prefix + " " + url + " " + suite + " " + rest
}

func releaseURLs(mirror, suite string) []string {
	base := strings.TrimRight(mirror, "/") + "/dists/" + suite
	return []string{base + "/InRelease", base + "/Release"}
}

func managedRepoList(name string) bool {
	n := strings.ToLower(name)
	return strings.HasPrefix(n, "cp-") || strings.Contains(n, "mariadb") || strings.Contains(n, "mysql") || strings.Contains(n, "nginx")
}

type repoProbe int

const (
	repoProbeUnknown repoProbe = iota
	repoProbeMissing
	repoProbeOK
)

func pickRepoSuite(mirror, id, code string, probe func(string, string) repoProbe) (suite string, disable bool) {
	unknown := true
	for _, s := range fallbackSuites(id, code) {
		switch probe(mirror, s) {
		case repoProbeOK:
			return s, false
		case repoProbeMissing:
			unknown = false
		}
	}
	if unknown {
		if strings.TrimSpace(code) == "" {
			return "noble", false
		}
		return code, false
	}
	return "", true
}

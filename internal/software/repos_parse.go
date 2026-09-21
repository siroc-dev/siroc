package software

import "strings"

var ubuntuVendorSuites = []string{"noble", "jammy", "focal"}
var debianVendorSuites = []string{"trixie", "bookworm", "bullseye"}

const suryPHPMirror = "https://packages.sury.org/php"

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

func launchpadPPA(url string) bool {
	u := strings.ToLower(url)
	return strings.Contains(u, "ppa.launchpadcontent.net") || strings.Contains(u, "ppa.launchpad.net")
}

func suryPHP(url string) bool {
	return strings.Contains(strings.ToLower(url), "packages.sury.org")
}

func sameSuiteVendor(url string) bool {
	return launchpadPPA(url) || suryPHP(url)
}

func launchpadPPAMirror(owner, name, distro string) string {
	if distro == "" {
		distro = "ubuntu"
	}
	return "https://ppa.launchpadcontent.net/" + owner + "/" + name + "/" + distro
}

func officialArchive(url string) bool {
	u := strings.ToLower(url)
	for _, h := range []string{
		"archive.ubuntu.com",
		"security.ubuntu.com",
		"ports.ubuntu.com",
		"clouds.archive.ubuntu.com",
		"azure.archive.ubuntu.com",
		"deb.debian.org",
		"security.debian.org",
		"debian.org/debian",
	} {
		if strings.Contains(u, h) {
			return true
		}
	}
	return false
}

func firstField(s string) string {
	f := strings.Fields(s)
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

func deb822Field(block, key string) string {
	prefix := strings.ToLower(key) + ":"
	for _, line := range strings.Split(block, "\n") {
		trim := strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(trim), prefix) {
			continue
		}
		_, val, ok := strings.Cut(trim, ":")
		if ok {
			return strings.TrimSpace(val)
		}
	}
	return ""
}

func replaceDeb822Field(block, key, value string) string {
	prefix := strings.ToLower(key) + ":"
	var out []string
	found := false
	for _, line := range strings.Split(block, "\n") {
		trim := strings.TrimSpace(line)
		if !found && strings.HasPrefix(strings.ToLower(trim), prefix) {
			lead := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			out = append(out, lead+key+": "+value)
			found = true
			continue
		}
		out = append(out, line)
	}
	if !found {
		out = append(out, key+": "+value)
	}
	return strings.Join(out, "\n")
}

func splitDeb822(text string) []string {
	return strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n")
}

func joinDeb822(blocks []string) string {
	return strings.Join(blocks, "\n\n")
}

type repoProbe int

const (
	repoProbeUnknown repoProbe = iota
	repoProbeMissing
	repoProbeOK
)

func pickRepoSuite(mirror, id, code string, probe func(string, string) repoProbe) (suite string, disable bool) {
	suites := fallbackSuites(id, code)
	if sameSuiteVendor(mirror) {
		if strings.TrimSpace(code) == "" {
			return "", true
		}
		suites = []string{code}
	}
	unknown := true
	for _, s := range suites {
		switch probe(mirror, s) {
		case repoProbeOK:
			return s, false
		case repoProbeMissing:
			unknown = false
		}
	}
	if unknown {
		if sameSuiteVendor(mirror) {
			return code, false
		}
		if strings.TrimSpace(code) == "" {
			return "noble", false
		}
		return code, false
	}
	return "", true
}

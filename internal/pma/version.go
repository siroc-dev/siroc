package pma

var phpVersions = []string{"8.4", "8.3", "8.2", "8.1"}

func pickPHPVersion(active, installed []string) string {
	has := func(list []string, want string) bool {
		for _, v := range list {
			if v == want {
				return true
			}
		}
		return false
	}
	for _, v := range phpVersions {
		if has(active, v) {
			return v
		}
	}
	for _, v := range phpVersions {
		if has(installed, v) {
			return v
		}
	}
	return ""
}

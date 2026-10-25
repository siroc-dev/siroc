package software

// PHPVersions can be installed side by side. 8.4 stays the default for a new server.
func PHPVersions() []string {
	return []string{"7.4", "8.1", "8.2", "8.3", "8.4"}
}

func SupportedPHP(v string) bool {
	for _, x := range PHPVersions() {
		if x == v {
			return true
		}
	}
	return false
}

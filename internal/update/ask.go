package update

import "strings"

// Accepted reports whether an operator agreed to install an update.
func Accepted(answer string) bool {
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// OfferInstall is true only for an interactive check that found a newer package.
// The automatic update timer passes applyFlag and must not be offered a prompt.
func OfferInstall(available, tty, applyFlag bool) bool {
	return available && tty && !applyFlag
}

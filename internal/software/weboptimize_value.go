package software

func webShownValue(live, recommend string, preset bool) string {
	if preset && recommend != "" {
		return recommend
	}
	if live != "" {
		return live
	}
	return recommend
}

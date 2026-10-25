package dbmgmt

func dbShownValue(live, fileVal, recommend string, preset bool) string {
	if preset && recommend != "" {
		return recommend
	}
	if live != "" {
		return live
	}
	if fileVal != "" {
		return fileVal
	}
	return recommend
}

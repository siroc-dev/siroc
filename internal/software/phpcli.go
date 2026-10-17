package software

// CLILimitsINI is loaded only by the PHP CLI SAPI. PHP-FPM keeps its own php.ini
// and the pool limits set for each website.
func CLILimitsINI() string {
	return "; Siroc: CLI scripts are not cut off by time or memory.\n" +
		"max_execution_time = 0\n" +
		"memory_limit = -1\n"
}

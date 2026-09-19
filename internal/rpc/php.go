package rpc

import (
	"fmt"
	"strings"
)

func DefaultPHPFPM() PHPFPMSettings {
	return PHPFPMSettings{
		PM:                "ondemand",
		MaxChildren:       20,
		StartServers:      2,
		MinSpare:          1,
		MaxSpare:          4,
		IdleTimeout:       "10s",
		MaxRequests:       500,
		MemoryLimit:       "256M",
		MaxExecutionTime:  60,
		MaxInputTime:      60,
		PostMaxSize:       "64M",
		UploadMaxFilesize: "64M",
		Timezone:          "UTC",
		DisableFunctions:  "exec,passthru,shell_exec,system,proc_open,popen,show_source",
		OpenBasedir:       true,
		Extensions:        map[string][]string{},
	}
}

func MergePHPFPM(in PHPFPMSettings) PHPFPMSettings {
	d := DefaultPHPFPM()
	if in.PM != "" {
		d.PM = in.PM
	}
	if in.MaxChildren > 0 {
		d.MaxChildren = in.MaxChildren
	}
	if in.StartServers > 0 {
		d.StartServers = in.StartServers
	}
	if in.MinSpare > 0 {
		d.MinSpare = in.MinSpare
	}
	if in.MaxSpare > 0 {
		d.MaxSpare = in.MaxSpare
	}
	if in.IdleTimeout != "" {
		d.IdleTimeout = in.IdleTimeout
	}
	if in.MaxRequests > 0 {
		d.MaxRequests = in.MaxRequests
	}
	if in.MemoryLimit != "" {
		d.MemoryLimit = in.MemoryLimit
	}
	if in.MaxExecutionTime > 0 {
		d.MaxExecutionTime = in.MaxExecutionTime
	}
	if in.MaxInputTime > 0 {
		d.MaxInputTime = in.MaxInputTime
	}
	if in.PostMaxSize != "" {
		d.PostMaxSize = in.PostMaxSize
	}
	if in.UploadMaxFilesize != "" {
		d.UploadMaxFilesize = in.UploadMaxFilesize
	}
	if in.Timezone != "" {
		d.Timezone = in.Timezone
	}
	d.DisplayErrors = in.DisplayErrors
	d.OpenBasedir = true
	d.DisableFunctions = in.DisableFunctions
	if in.Extensions != nil {
		d.Extensions = in.Extensions
	}
	return d
}

func ParsePHPExtJob(version string) (ver, name, source string, err error) {
	parts := strings.Split(strings.TrimSpace(version), ":")
	if len(parts) < 2 || len(parts) > 3 {
		return "", "", "", fmt.Errorf("php-ext job must be phpVer:extension[:apt|pecl]")
	}
	ver = strings.TrimSpace(parts[0])
	name = strings.ToLower(strings.TrimSpace(parts[1]))
	source = "auto"
	if len(parts) == 3 {
		source = strings.ToLower(strings.TrimSpace(parts[2]))
	}
	if ver == "" || name == "" {
		return "", "", "", fmt.Errorf("php-ext job must be phpVer:extension[:apt|pecl]")
	}
	switch source {
	case "apt", "pecl", "auto":
	default:
		return "", "", "", fmt.Errorf("php-ext source must be apt, pecl, or auto")
	}
	return ver, name, source, nil
}

func PHPExtJobSpec(ver, name, source string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" {
		source = "auto"
	}
	return strings.TrimSpace(ver) + ":" + strings.ToLower(strings.TrimSpace(name)) + ":" + source
}

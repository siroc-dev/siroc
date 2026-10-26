//go:build !linux

package diskspace

func readMounts() []Mount { return nil }

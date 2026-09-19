//go:build linux

package software

import "github.com/siroc-dev/siroc/internal/pma"

func pmaInstalled() (bool, string) { return pma.Installed() }

func pmaSetup() error { return pma.Setup() }

func pmaRefresh() error { return pma.Refresh() }

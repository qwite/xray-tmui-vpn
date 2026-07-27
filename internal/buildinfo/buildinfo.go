package buildinfo

import (
	"fmt"
	"strings"
)

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func DisplayVersion() string {
	version := strings.TrimSpace(Version)
	if version == "" || version == "dev" {
		return "dev"
	}
	if strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}

func String() string {
	return fmt.Sprintf(
		"xray-tmui-vpn %s (commit %s, built %s)",
		DisplayVersion(),
		valueOr(Commit, "none"),
		valueOr(Date, "unknown"),
	)
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

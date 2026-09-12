package project

import (
	"runtime/debug"
	"strings"
)

// CurrentApplicationVersion returns the module version or a development identity.
func CurrentApplicationVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "development"
	}
	version := strings.TrimSpace(info.Main.Version)
	if version != "" && version != "(devel)" {
		return version
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" && strings.TrimSpace(setting.Value) != "" {
			revision := strings.TrimSpace(setting.Value)
			if len(revision) > 12 {
				revision = revision[:12]
			}
			return "development+" + revision
		}
	}
	return "development"
}

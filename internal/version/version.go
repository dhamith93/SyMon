// Package version is the SyMon version a binary was built from
package version

import "runtime/debug"

// Version is set at build time, for example
//
//	go build -ldflags "-X github.com/dhamith93/SyMon/internal/version.Version=v3.1.0"
//
// The Makefile sets it from git describe.
var Version = ""

// String returns Version. A binary built without it, like with a plain go
// build, reports the git commit it was built from, and "dev" without one.
func String() string {
	if Version != "" {
		return Version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	return fromBuildInfo(info.Settings)
}

func fromBuildInfo(settings []debug.BuildSetting) string {
	revision, modified := "", false
	for _, setting := range settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if revision == "" {
		return "dev"
	}
	if len(revision) > 7 {
		revision = revision[:7]
	}
	if modified {
		revision += "-dirty"
	}
	return revision
}

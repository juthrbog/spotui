// Package version holds the version of spotui.
package version

import (
	"fmt"
	"runtime/debug"
	"strings"
)

// The release build sets these with -ldflags -X.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// String returns a line such as
// "spotui 0.1.0 (commit 1a2b3c4, built 2026-10-16T12:00:00Z)".
//
// A binary from "go install" has no -ldflags. It gets the module version
// and, in a git checkout, the commit from the Go build information.
func String() string {
	return format(Version, Commit, Date, readBuildInfo())
}

func readBuildInfo() *debug.BuildInfo {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return nil
	}
	return info
}

func format(v, commit, date string, info *debug.BuildInfo) string {
	if info != nil {
		if v == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
			v = strings.TrimPrefix(info.Main.Version, "v")
		}
		for _, s := range info.Settings {
			switch {
			case s.Key == "vcs.revision" && commit == "none" && len(s.Value) >= 7:
				commit = s.Value[:7]
			case s.Key == "vcs.time" && date == "unknown":
				date = s.Value
			}
		}
	}
	return fmt.Sprintf("spotui %s (commit %s, built %s)", v, commit, date)
}

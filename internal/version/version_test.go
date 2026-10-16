package version

import (
	"runtime/debug"
	"testing"
)

func TestFormat(t *testing.T) {
	installed := &debug.BuildInfo{Main: debug.Module{Version: "v0.2.0"}}
	checkout := &debug.BuildInfo{
		Main: debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "1a2b3c4d5e6f"},
			{Key: "vcs.time", Value: "2026-10-16T10:00:00Z"},
		},
	}
	cases := []struct {
		name            string
		v, commit, date string
		info            *debug.BuildInfo
		want            string
	}{
		{"release build", "0.1.0", "abc1234", "2026-10-16T12:00:00Z", installed,
			"spotui 0.1.0 (commit abc1234, built 2026-10-16T12:00:00Z)"},
		{"go install", "dev", "none", "unknown", installed,
			"spotui 0.2.0 (commit none, built unknown)"},
		{"git checkout", "dev", "none", "unknown", checkout,
			"spotui dev (commit 1a2b3c4, built 2026-10-16T10:00:00Z)"},
		{"no build info", "dev", "none", "unknown", nil,
			"spotui dev (commit none, built unknown)"},
	}
	for _, c := range cases {
		if got := format(c.v, c.commit, c.date, c.info); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

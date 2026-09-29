package version

import (
	"runtime/debug"
	"testing"
)

func TestString(t *testing.T) {
	Version = "v3.1.0"
	t.Cleanup(func() { Version = "" })
	if got := String(); got != "v3.1.0" {
		t.Errorf("expected the version set at build time, got %q", got)
	}
}

func TestFromBuildInfo(t *testing.T) {
	commit := debug.BuildSetting{Key: "vcs.revision", Value: "2519821d4b5c6e0f1a2b3c4d5e6f7a8b9c0d1e2f"}
	tests := []struct {
		settings []debug.BuildSetting
		want     string
	}{
		{nil, "dev"},
		{[]debug.BuildSetting{commit}, "2519821"},
		{[]debug.BuildSetting{commit, {Key: "vcs.modified", Value: "true"}}, "2519821-dirty"},
	}
	for _, tt := range tests {
		if got := fromBuildInfo(tt.settings); got != tt.want {
			t.Errorf("%v: got %q, want %q", tt.settings, got, tt.want)
		}
	}
}

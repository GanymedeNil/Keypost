// Package version holds the release number of this build.
//
// The number lives in the VERSION file next to this source (one line, semantic
// versioning, no leading "v") and is embedded at compile time, so every way of
// building the binaries (go run, go build, docker build) reports the same value.
package version

import (
	_ "embed"
	"runtime/debug"
	"strings"
)

//go:embed VERSION
var raw string

// Version is the release number, for example "0.1.0".
func Version() string { return strings.TrimSpace(raw) }

// Commit is the short VCS revision the Go toolchain stamped into the binary,
// with a "-dirty" suffix for uncommitted changes. It is empty when the build
// has no VCS information (for example a docker build without a .git directory).
func Commit() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	var rev string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	if rev != "" && dirty {
		rev += "-dirty"
	}
	return rev
}

// String is the number plus the commit when known: "0.1.0 (a1b2c3d)".
func String() string {
	if c := Commit(); c != "" {
		return Version() + " (" + c + ")"
	}
	return Version()
}

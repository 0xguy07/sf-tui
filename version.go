package main

import (
	"fmt"
	"runtime/debug"
)

// Set at release time by GoReleaser via -ldflags "-X main.version=...".
var (
	version = "dev"
	commit  = ""
	date    = ""
)

// versionString describes this build. Release builds use the injected
// values; `go install ...@vX.Y.Z` builds fall back to the module version
// recorded in the binary.
func versionString() string {
	v := version
	if v == "dev" {
		if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			v = bi.Main.Version
		}
	}
	s := "sftui " + v
	if commit != "" {
		s += fmt.Sprintf(" (%s", commit)
		if date != "" {
			s += ", " + date
		}
		s += ")"
	}
	return s
}

// wantsVersion reports whether the first argument asks for the version.
// Only the first argument counts, so `sftui why -v` is not a version request.
func wantsVersion(args []string) bool {
	if len(args) == 0 {
		return false
	}
	a := args[0]
	return a == "--version" || a == "-v" || a == "version"
}

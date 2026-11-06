package main

import (
	"strings"
	"testing"
)

func TestWantsVersion(t *testing.T) {
	for _, args := range [][]string{{"--version"}, {"-v"}, {"version"}} {
		if !wantsVersion(args) {
			t.Errorf("wantsVersion(%v) = false", args)
		}
	}
	for _, args := range [][]string{nil, {}, {"--help"}, {"query"}} {
		if wantsVersion(args) {
			t.Errorf("wantsVersion(%v) = true", args)
		}
	}
}

func TestVersionString(t *testing.T) {
	oldV, oldC, oldD := version, commit, date
	defer func() { version, commit, date = oldV, oldC, oldD }()

	version, commit, date = "1.2.3", "abc1234", "2026-11-06"
	if got, want := versionString(), "sftui 1.2.3 (abc1234, 2026-11-06)"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	version, commit, date = "1.2.3", "", ""
	if got, want := versionString(), "sftui 1.2.3"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	version = "dev"
	if !strings.HasPrefix(versionString(), "sftui ") {
		t.Errorf("dev build should still print a name, got %q", versionString())
	}
}

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunCLI_NoArgsIsNotHandled(t *testing.T) {
	var out, errb bytes.Buffer
	handled, code := runCLI(nil, &out, &errb, cliDeps{})
	if handled || code != 0 {
		t.Fatalf("got handled=%v code=%d, want false/0", handled, code)
	}
	if out.Len() != 0 || errb.Len() != 0 {
		t.Errorf("expected no output, got stdout=%q stderr=%q", out.String(), errb.String())
	}
}

func TestRunCLI_VersionFirstArgOnly(t *testing.T) {
	var out, errb bytes.Buffer
	handled, code := runCLI([]string{"--version"}, &out, &errb, cliDeps{})
	if !handled || code != 0 {
		t.Fatalf("got handled=%v code=%d", handled, code)
	}
	if got, want := out.String(), versionString()+"\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestRunCLI_UnknownSubcommand(t *testing.T) {
	var out, errb bytes.Buffer
	handled, code := runCLI([]string{"frobnicate"}, &out, &errb, cliDeps{})
	if !handled || code != 2 {
		t.Fatalf("got handled=%v code=%d, want true/2", handled, code)
	}
	if !strings.Contains(strings.ToLower(errb.String()), "usage") {
		t.Errorf("stderr should contain usage, got %q", errb.String())
	}
	if out.Len() != 0 {
		t.Errorf("stdout should be empty, got %q", out.String())
	}
}

func TestRunCLI_HelpOnStdout(t *testing.T) {
	var out, errb bytes.Buffer
	handled, code := runCLI([]string{"--help"}, &out, &errb, cliDeps{})
	if !handled || code != 0 {
		t.Fatalf("got handled=%v code=%d, want true/0", handled, code)
	}
	if !strings.Contains(out.String(), "sftui why") {
		t.Errorf("stdout should mention 'sftui why', got %q", out.String())
	}
}

func TestUsage_CompareTakesNoOrg(t *testing.T) {
	u := usage()
	if !strings.Contains(u, "compare takes no --org") {
		t.Errorf("usage should say compare takes no --org:\n%s", u)
	}
	if strings.Contains(u, "common flags") {
		t.Error("flags should be listed per command, not as 'common'")
	}
}

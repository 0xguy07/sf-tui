package main

import (
	"fmt"
	"io"

	"github.com/0xguy07/sf-tui/sf"
)

// cliDeps holds everything the non-interactive commands need from the
// outside world, so tests can run them offline. Nothing here can write to
// an org: the CLI is read-only by construction.
type cliDeps struct {
	trace      func(org, id string, refresh bool) (sf.Trace, error)
	defaultOrg func() (string, error)
}

// prodDeps wires the real Salesforce access used by `sftui why`.
func prodDeps() cliDeps {
	return cliDeps{
		trace: func(org, id string, refresh bool) (sf.Trace, error) {
			return sf.BuildTrace(sf.RESTFor(org), org, id, refresh)
		},
		defaultOrg: sf.DefaultTargetOrg,
	}
}

func usage() string {
	return `Usage:
  sftui                         start the TUI
  sftui why <recordId> [flags]  explain why a record changed (read-only)
  sftui --version               print the version

why flags:
  --org, -o <alias|username>    org to query (default: the sf default target org)
  --format <text|json>          output format (default: text)
  --refresh                     bypass the 24-hour automation cache
`
}

// runCLI handles non-interactive invocations. It reports handled=false
// when the arguments mean "start the TUI".
func runCLI(args []string, stdout, stderr io.Writer, d cliDeps) (handled bool, code int) {
	if len(args) == 0 {
		return false, 0
	}
	if wantsVersion(args) {
		fmt.Fprintln(stdout, versionString())
		return true, 0
	}
	switch args[0] {
	case "help", "--help", "-h":
		fmt.Fprint(stdout, usage())
		return true, 0
	case "why":
		return true, runWhy(args[1:], stdout, stderr, d)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage())
		return true, 2
	}
}

// runWhy is implemented in cli_why.go.
func runWhy(args []string, stdout, stderr io.Writer, d cliDeps) int {
	fmt.Fprintln(stderr, "why: not implemented")
	return 2
}

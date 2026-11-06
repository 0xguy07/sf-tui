package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/0xguy07/sf-tui/sf"
)

// cliDeps holds everything the non-interactive commands need from the
// outside world, so tests can run them offline. Nothing here can write to
// an org: the CLI is read-only by construction.
type cliDeps struct {
	trace       func(org, id string, refresh bool) (sf.Trace, error)
	defaultOrg  func() (string, error)
	resolveUser func(org, ident string) (sf.UserBrief, error)
	perms       func(org string, user sf.UserBrief, object string) (sf.PermissionsReport, error)
}

// prodDeps wires the real Salesforce access used by `sftui why`.
func prodDeps() cliDeps {
	return cliDeps{
		trace: func(org, id string, refresh bool) (sf.Trace, error) {
			return sf.BuildTrace(sf.RESTFor(org), org, id, refresh)
		},
		defaultOrg: sf.DefaultTargetOrg,
		resolveUser: func(org, ident string) (sf.UserBrief, error) {
			return sf.ResolveUser(sf.DefaultSOQLRunner(), org, ident)
		},
		perms: func(org string, user sf.UserBrief, object string) (sf.PermissionsReport, error) {
			return sf.BuildPermissionsReport(sf.DefaultSOQLRunner(), org, user, object)
		},
	}
}

// resolveOrg picks the org to query: the explicit flag, else the sf default
// target org. Returns a non-zero exit code when none can be found.
func resolveOrg(explicit string, d cliDeps, stderr io.Writer) (string, int) {
	org := explicit
	if org == "" && d.defaultOrg != nil {
		dflt, err := d.defaultOrg()
		if err != nil {
			// The sf CLI itself failed (missing, broken): a runtime failure, not a usage error.
			fmt.Fprintf(stderr, "%v\n", err)
			return "", 1
		}
		org = dflt
	}
	if org == "" {
		fmt.Fprintln(stderr, "no org: pass --org <alias> or run: sf config set target-org=<alias>")
		return "", 2
	}
	return org, 0
}

// writeReport renders v as JSON or via text and prints section warnings.
func writeReport(stdout, stderr io.Writer, format string, v any, text func() string, errs []sf.SectionError) int {
	if format == "json" {
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "encode: %v\n", err)
			return 1
		}
		stdout.Write(append(b, '\n'))
	} else {
		io.WriteString(stdout, text())
	}
	for _, e := range errs {
		fmt.Fprintf(stderr, "warning: %s: %s\n", e.Section, e.Message)
	}
	return 0
}

func usage() string {
	return `Usage:
  sftui                         start the TUI
  sftui why <recordId> [flags]  explain why a record changed (read-only)
  sftui perms <user> [flags]    what a user can do, and which perm set grants it (read-only)
  sftui --version               print the version

common flags:
  --org, -o <alias|username>    org to query (default: the sf default target org)
  --format <text|json>          output format (default: text)

why flags:
  --refresh                     bypass the 24-hour automation cache

perms flags:
  --object <SObject>            also show field-level permissions on this object

<user> may be an Id (005...), a username, or an exact display name.
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
	case "perms":
		return true, runPerms(args[1:], stdout, stderr, d)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage())
		return true, 2
	}
}

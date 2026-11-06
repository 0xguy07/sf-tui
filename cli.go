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
	compare     func(aOrg, bOrg string, opts sf.CompareOptions) (sf.SchemaDiff, error)
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
		compare: func(aOrg, bOrg string, opts sf.CompareOptions) (sf.SchemaDiff, error) {
			return sf.BuildSchemaDiff(sf.DefaultSchemaLoader(), aOrg, bOrg, opts)
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
// Warnings go to stderr unprefixed, as "warning: <section>: <message>", so
// scripts can match them the same way for every command; errw is the
// command-prefixed writer used for failures.
func writeReport(stdout, stderr, errw io.Writer, format string, v any, text func() string, errs []sf.SectionError) int {
	if format == "json" {
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			fmt.Fprintf(errw, "encode: %v\n", err)
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
  sftui compare <orgA> <orgB>   schema diff between two orgs (read-only)
  sftui --version               print the version

why flags:
  --org, -o <alias|username>    org to query (default: the sf default target org)
  --format <text|json>          output format (default: text)
  --refresh                     bypass the 24-hour automation cache
  --timeout <duration>          deadline per sf call, e.g. 90s or 2m (default 10m; deploys/tests 40m; or SFTUI_TIMEOUT)

perms flags:
  --org, -o <alias|username>    org to query (default: the sf default target org)
  --format <text|json>          output format (default: text)
  --object <SObject>            also show field-level permissions on this object
  --timeout <duration>          deadline per sf call (default 10m, or SFTUI_TIMEOUT)

compare flags (compare takes no --org: both orgs are positional):
  --format <text|json>          output format (default: text)
  --object <SObject>            field-level diff for one object
  --fields                      field-level diff for every object present on both sides
  --custom-only                 with --fields: custom objects only
  --exit-code                   exit 3 when differences are found
  --refresh                     bypass the 24-hour schema caches
  --timeout <duration>          deadline per sf call (default 10m, or SFTUI_TIMEOUT)

<user> may be an Id (005...), a username, or an exact display name.
`
}

// applyTimeoutEnv applies SFTUI_TIMEOUT when set and valid. It returns a
// warning to show the user when the value is invalid (the default stays).
func applyTimeoutEnv() string {
	d, ok, err := sf.TimeoutFromEnv()
	switch {
	case err != nil:
		return fmt.Sprintf("%v; using the default %s", err, sf.DefaultTimeout)
	case ok:
		sf.SetTimeout(d)
	}
	return ""
}

// applyTimeoutFlag applies a --timeout value, if given. Returns 2 on a bad value.
func applyTimeoutFlag(v string, errw io.Writer) int {
	if v == "" {
		return 0
	}
	d, err := sf.ParseTimeout(v)
	if err != nil {
		fmt.Fprintf(errw, "--timeout %q: %v\n", v, err)
		return 2
	}
	sf.SetTimeout(d)
	return 0
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
	}
	if w := applyTimeoutEnv(); w != "" {
		fmt.Fprintln(stderr, "warning: "+w)
	}
	switch args[0] {
	case "why":
		return true, runWhy(args[1:], stdout, stderr, d)
	case "perms":
		return true, runPerms(args[1:], stdout, stderr, d)
	case "compare":
		return true, runCompare(args[1:], stdout, stderr, d)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage())
		return true, 2
	}
}

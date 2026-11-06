package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/0xguy07/sf-tui/sf"
)

// runWhy implements `sftui why <recordId> [--org] [--format text|json] [--refresh]`.
// Exit codes: 0 success (including partial results), 1 runtime failure, 2 usage.
func runWhy(args []string, stdout, stderr io.Writer, d cliDeps) int {
	id, rest := splitPositional(args)

	fs := flag.NewFlagSet("why", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {} // printed by hand below so help goes to stdout and errors to stderr
	var org, format string
	var refresh bool
	fs.StringVar(&org, "org", "", "org alias or username")
	fs.StringVar(&org, "o", "", "org alias or username (shorthand)")
	fs.StringVar(&format, "format", "text", "text or json")
	fs.BoolVar(&refresh, "refresh", false, "bypass the automation cache")
	if err := fs.Parse(rest); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage())
			return 0
		}
		fmt.Fprint(stderr, "\n"+usage())
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "why: unexpected argument %q (one record Id only)\n", fs.Arg(0))
		return 2
	}

	id = strings.TrimSpace(id)
	if id == "" {
		fmt.Fprint(stderr, "why: missing record Id\n\n"+usage())
		return 2
	}
	if !sf.ValidRecordID(id) {
		fmt.Fprintf(stderr, "why: invalid record Id: %q (expected 15 or 18 alphanumeric characters)\n", id)
		return 2
	}
	format = strings.ToLower(format)
	if format != "text" && format != "json" {
		fmt.Fprintf(stderr, "why: --format must be text or json, got %q\n", format)
		return 2
	}

	if org == "" && d.defaultOrg != nil {
		dflt, err := d.defaultOrg()
		if err != nil {
			// The sf CLI itself failed (missing, broken): a runtime failure, not a usage error.
			fmt.Fprintf(stderr, "why: %v\n", err)
			return 1
		}
		org = dflt
	}
	if org == "" {
		fmt.Fprintln(stderr, "why: no org: pass --org <alias> or run: sf config set target-org=<alias>")
		return 2
	}

	tr, err := d.trace(org, id, refresh)
	if err != nil {
		fmt.Fprintf(stderr, "why: %v\n", err)
		return 1
	}

	switch format {
	case "json":
		b, err := json.MarshalIndent(tr, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "why: encode: %v\n", err)
			return 1
		}
		stdout.Write(append(b, '\n'))
	default:
		io.WriteString(stdout, renderTraceText(tr))
	}
	for _, e := range tr.Errors {
		fmt.Fprintf(stderr, "warning: %s: %s\n", e.Section, e.Message)
	}
	return 0
}

// splitPositional pulls the first non-flag argument out as the record Id so
// it may appear before or after the flags. "--" ends flag parsing: everything
// after it is positional. Any positional beyond the Id is handed back after a
// "--" so flag.Parse leaves it in fs.Args() for the caller to reject.
func splitPositional(args []string) (id string, rest []string) {
	takesValue := map[string]bool{"--org": true, "-org": true, "--o": true, "-o": true, "--format": true, "-format": true}
	var extra []string
	take := func(a string) {
		if id == "" {
			id = a
		} else {
			extra = append(extra, a)
		}
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			for _, p := range args[i+1:] {
				take(p)
			}
			break
		}
		if strings.HasPrefix(a, "-") {
			rest = append(rest, a)
			if takesValue[a] && i+1 < len(args) {
				i++
				rest = append(rest, args[i])
			}
			continue
		}
		take(a)
	}
	if len(extra) > 0 {
		rest = append(rest, "--")
		rest = append(rest, extra...)
	}
	return id, rest
}

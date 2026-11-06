package main

import (
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
	var org, format, timeout string
	var refresh bool
	fs.StringVar(&org, "org", "", "org alias or username")
	fs.StringVar(&org, "o", "", "org alias or username (shorthand)")
	fs.StringVar(&format, "format", "text", "text or json")
	fs.BoolVar(&refresh, "refresh", false, "bypass the automation cache")
	fs.StringVar(&timeout, "timeout", "", "deadline per sf call")
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

	if code := applyTimeoutFlag(timeout, prefixWriter{stderr, "why: "}); code != 0 {
		return code
	}
	org, code := resolveOrg(org, d, prefixWriter{stderr, "why: "})
	if code != 0 {
		return code
	}

	tr, err := d.trace(org, id, refresh)
	if err != nil {
		fmt.Fprintf(stderr, "why: %s in %s: %v\n", id, org, err)
		return 1
	}

	return writeReport(stdout, stderr, prefixWriter{stderr, "why: "}, format, tr, func() string { return renderTraceText(tr) }, tr.Errors)
}

// prefixWriter prefixes each write with a command name, for stderr lines.
type prefixWriter struct {
	w      io.Writer
	prefix string
}

func (p prefixWriter) Write(b []byte) (int, error) {
	if _, err := io.WriteString(p.w, p.prefix); err != nil {
		return 0, err
	}
	return p.w.Write(b)
}

// splitPositional pulls the first non-flag argument out as the positional so
// it may appear before or after the flags. See splitPositionalN.
func splitPositional(args []string) (id string, rest []string) {
	pos, rest := splitPositionalN(args, 1)
	if len(pos) > 0 {
		id = pos[0]
	}
	return id, rest
}

// splitPositionalN pulls up to n non-flag arguments out of args so they may
// appear before or after the flags. "--" ends flag parsing: everything after
// it is positional. Positionals beyond n are handed back after a "--" so
// flag.Parse leaves them in fs.Args() for the caller to reject.
func splitPositionalN(args []string, n int) (positionals []string, rest []string) {
	takesValue := map[string]bool{"--org": true, "-org": true, "--o": true, "-o": true, "--format": true, "-format": true, "--object": true, "-object": true, "--timeout": true, "-timeout": true}
	var extra []string
	take := func(a string) {
		if len(positionals) < n {
			positionals = append(positionals, a)
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
	return positionals, rest
}

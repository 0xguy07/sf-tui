package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/0xguy07/sf-tui/sf"
)

// runCompare implements `sftui compare <orgA> <orgB> [--object X | --fields
// [--custom-only]] [--exit-code] [--refresh] [--format text|json]`.
// Exit codes: 0 success, 1 runtime failure, 2 usage, 3 differences with --exit-code.
func runCompare(args []string, stdout, stderr io.Writer, d cliDeps) int {
	orgs, rest := splitPositionalN(args, 2)

	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	var object, format string
	var fields, customOnly, exitCode, refresh bool
	fs.StringVar(&object, "object", "", "field-level diff for one object")
	fs.BoolVar(&fields, "fields", false, "field-level diff for every object on both sides")
	fs.BoolVar(&customOnly, "custom-only", false, "with --fields: only custom objects")
	fs.BoolVar(&exitCode, "exit-code", false, "exit 3 when differences are found")
	fs.BoolVar(&refresh, "refresh", false, "bypass the on-disk caches")
	fs.StringVar(&format, "format", "text", "text or json")
	if err := fs.Parse(rest); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage())
			return 0
		}
		fmt.Fprint(stderr, "\n"+usage())
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "compare: unexpected argument %q (two orgs only)\n", fs.Arg(0))
		return 2
	}
	if len(orgs) != 2 || strings.TrimSpace(orgs[0]) == "" || strings.TrimSpace(orgs[1]) == "" {
		fmt.Fprint(stderr, "compare: need two orgs: sftui compare <orgA> <orgB>\n\n"+usage())
		return 2
	}
	a, b := strings.TrimSpace(orgs[0]), strings.TrimSpace(orgs[1])
	if a == b {
		fmt.Fprintf(stderr, "compare: both sides are the same org (%s)\n", a)
		return 2
	}
	object = strings.TrimSpace(object)
	if object != "" && fields {
		fmt.Fprintln(stderr, "compare: --object and --fields are mutually exclusive")
		return 2
	}
	if customOnly && !fields {
		fmt.Fprintln(stderr, "compare: --custom-only requires --fields")
		return 2
	}
	format = strings.ToLower(format)
	if format != "text" && format != "json" {
		fmt.Fprintf(stderr, "compare: --format must be text or json, got %q\n", format)
		return 2
	}

	errw := prefixWriter{stderr, "compare: "}
	opts := sf.CompareOptions{Object: object, Fields: fields, CustomOnly: customOnly, Refresh: refresh, Workers: 4}
	diff, err := d.compare(a, b, opts)
	if err != nil {
		fmt.Fprintf(errw, "%v\n", err)
		return 1
	}
	if code := writeReport(stdout, stderr, errw, format, diff, func() string { return renderSchemaDiffText(diff) }, diff.Errors); code != 0 {
		return code
	}
	if exitCode && diff.Different {
		return 3
	}
	return 0
}

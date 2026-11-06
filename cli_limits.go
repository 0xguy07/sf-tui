package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/0xguy07/sf-tui/sf"
)

// runLimits implements `sftui limits [--org] [--warn N] [--format text|json]`.
// Exit codes: 0 success, 1 runtime failure, 2 usage, 3 a limit is at or above --warn.
func runLimits(args []string, stdout, stderr io.Writer, d cliDeps) int {
	pos, rest := splitPositionalN(args, 0)

	fs := flag.NewFlagSet("limits", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	var org, format, warnStr, timeout string
	fs.StringVar(&org, "org", "", "org alias or username")
	fs.StringVar(&org, "o", "", "org alias or username (shorthand)")
	fs.StringVar(&format, "format", "text", "text or json")
	fs.StringVar(&warnStr, "warn", "", "percent threshold, 1-100")
	fs.StringVar(&timeout, "timeout", "", "deadline per sf call")
	if err := fs.Parse(rest); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage())
			return 0
		}
		fmt.Fprint(stderr, "\n"+usage())
		return 2
	}
	if len(pos) > 0 || fs.NArg() > 0 {
		extra := append(pos, fs.Args()...)
		fmt.Fprintf(stderr, "limits: unexpected argument %q\n", extra[0])
		return 2
	}
	format = strings.ToLower(format)
	if format != "text" && format != "json" {
		fmt.Fprintf(stderr, "limits: --format must be text or json, got %q\n", format)
		return 2
	}
	var warn *int
	if warnStr != "" {
		n, err := strconv.Atoi(warnStr)
		if err != nil || n < 1 || n > 100 {
			fmt.Fprintf(stderr, "limits: --warn must be a whole number from 1 to 100, got %q\n", warnStr)
			return 2
		}
		warn = &n
	}
	errw := prefixWriter{stderr, "limits: "}
	if code := applyTimeoutFlag(timeout, errw); code != 0 {
		return code
	}
	org, code := resolveOrg(org, d, errw)
	if code != 0 {
		return code
	}

	limits, err := d.limits(org)
	if err != nil {
		fmt.Fprintf(errw, "%v\n", err)
		return 1
	}
	r := sf.BuildLimitsReport(limits, org, warn)
	if code := writeReport(stdout, stderr, errw, format, r, func() string { return renderLimitsText(r) }, nil); code != 0 {
		return code
	}
	if warn != nil && len(r.Breached) > 0 {
		return 3
	}
	return 0
}

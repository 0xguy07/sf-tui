package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/0xguy07/sf-tui/sf"
)

// runPerms implements `sftui perms <user> [--org] [--object X] [--format text|json]`.
// Exit codes: 0 success (including partial results), 1 runtime failure, 2 usage.
func runPerms(args []string, stdout, stderr io.Writer, d cliDeps) int {
	ident, rest := splitPositional(args)

	fs := flag.NewFlagSet("perms", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	var org, object, format string
	fs.StringVar(&org, "org", "", "org alias or username")
	fs.StringVar(&org, "o", "", "org alias or username (shorthand)")
	fs.StringVar(&object, "object", "", "sobject for field-level permissions")
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
		fmt.Fprintf(stderr, "perms: unexpected argument %q (one user only)\n", fs.Arg(0))
		return 2
	}

	ident = strings.TrimSpace(ident)
	if ident == "" {
		fmt.Fprint(stderr, "perms: missing user\n\n"+usage())
		return 2
	}
	format = strings.ToLower(format)
	if format != "text" && format != "json" {
		fmt.Fprintf(stderr, "perms: --format must be text or json, got %q\n", format)
		return 2
	}
	errw := prefixWriter{stderr, "perms: "}
	org, code := resolveOrg(org, d, errw)
	if code != 0 {
		return code
	}

	user, err := d.resolveUser(org, ident)
	if err != nil {
		fmt.Fprintf(errw, "%v\n", err)
		var amb *sf.ErrAmbiguousUser
		if errors.Is(err, sf.ErrUserNotFound) || errors.As(err, &amb) {
			return 2
		}
		return 1
	}

	r, err := d.perms(org, user, strings.TrimSpace(object))
	if err != nil {
		fmt.Fprintf(errw, "%v\n", err)
		return 1
	}
	if r.Fields == nil {
		r.Fields = []sf.FieldPerm{} // never null in JSON
	}
	return writeReport(stdout, stderr, errw, format, r, func() string { return renderPermissionsText(r) }, r.Errors)
}

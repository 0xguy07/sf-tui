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
	var org, object, format, timeout, diffIdent string
	fs.StringVar(&timeout, "timeout", "", "deadline per sf call")
	fs.StringVar(&diffIdent, "diff", "", "compare against this user")
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
	if code := applyTimeoutFlag(timeout, errw); code != 0 {
		return code
	}
	org, code := resolveOrg(org, d, errw)
	if code != 0 {
		return code
	}

	diffSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "diff" {
			diffSet = true
		}
	})
	diffIdent = strings.TrimSpace(diffIdent)
	if diffSet && diffIdent == "" {
		fmt.Fprintln(errw, "--diff needs a user (Id, username or exact name)")
		return 2
	}

	resolve := func(ident string) (sf.UserBrief, int) {
		u, err := d.resolveUser(org, ident)
		if err != nil {
			fmt.Fprintf(errw, "%v\n", err)
			var amb *sf.ErrAmbiguousUser
			if errors.Is(err, sf.ErrUserNotFound) || errors.As(err, &amb) {
				return u, 2
			}
			return u, 1
		}
		return u, 0
	}
	user, code := resolve(ident)
	if code != 0 {
		return code
	}
	var other sf.UserBrief
	if diffIdent != "" {
		// Resolve both users before fetching anything, so a bad --diff fails fast.
		if other, code = resolve(diffIdent); code != 0 {
			return code
		}
		if other.ID == user.ID {
			fmt.Fprintf(errw, "--diff names the same user (%s)\n", user.Username)
			return 2
		}
	}

	object = strings.TrimSpace(object)
	r, err := d.perms(org, user, object)
	if err != nil {
		fmt.Fprintf(errw, "%v\n", err)
		return 1
	}
	if r.Fields == nil {
		r.Fields = []sf.FieldPerm{} // never null in JSON
	}
	if diffIdent != "" {
		rb, err := d.perms(org, other, object)
		if err != nil {
			fmt.Fprintf(errw, "%v\n", err)
			return 1
		}
		diff := sf.DiffPermissions(r, rb)
		return writeReport(stdout, stderr, errw, format, diff, func() string { return renderPermissionsDiffText(diff) }, diff.Errors)
	}
	return writeReport(stdout, stderr, errw, format, r, func() string { return renderPermissionsText(r) }, r.Errors)
}

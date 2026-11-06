package main

import (
	"fmt"
	"strings"

	"github.com/0xguy07/sf-tui/sf"
)

// renderSchemaDiffText renders a SchemaDiff as plain text with no ANSI
// escapes, for `sftui compare --format text`.
func renderSchemaDiffText(d sf.SchemaDiff) string {
	var b strings.Builder
	mark := func(cached bool) string {
		if cached {
			return "  (cached)"
		}
		return ""
	}
	b.WriteString("Schema compare\n")
	fmt.Fprintf(&b, "  A: %s  (%d sobjects)%s\n", d.A.Org, d.A.SObjects, mark(d.A.Cached))
	fmt.Fprintf(&b, "  B: %s  (%d sobjects)%s\n\n", d.B.Org, d.B.SObjects, mark(d.B.Cached))

	fmt.Fprintf(&b, "Objects: %d only in A, %d only in B, %d in both\n\n", d.Summary.OnlyA, d.Summary.OnlyB, d.Summary.Both)
	writeObjects := func(title, glyph, state string) {
		var lines []string
		for _, o := range d.Objects {
			if o.State == state {
				line := "  " + glyph + " " + o.Name
				if o.Custom {
					line += "  (custom)"
				}
				lines = append(lines, line)
			}
		}
		if len(lines) > 0 {
			fmt.Fprintf(&b, "%s\n%s\n\n", title, strings.Join(lines, "\n"))
		}
	}
	writeObjects("Only in A:", "-", sf.StateOnlyA)
	writeObjects("Only in B:", "+", sf.StateOnlyB)

	if d.Summary.ObjectsCompared > 0 || len(d.Fields) > 0 {
		fmt.Fprintf(&b, "Fields: %d objects compared, %d with differences\n\n", d.Summary.ObjectsCompared, d.Summary.ObjectsChanged)
		for _, f := range d.Fields {
			if !f.Different {
				if d.Summary.ObjectsCompared == 1 {
					fmt.Fprintf(&b, "%s: no field-level differences (%d fields)\n\n", f.SObject, f.Same)
				}
				continue
			}
			fmt.Fprintf(&b, "%s\n", f.SObject)
			for _, x := range f.OnlyA {
				fmt.Fprintf(&b, "  - %s  %s\n", x.Name, x.Type)
			}
			for _, x := range f.OnlyB {
				fmt.Fprintf(&b, "  + %s  %s\n", x.Name, x.Type)
			}
			for _, c := range f.Changed {
				fmt.Fprintf(&b, "  ~ %s\n", c.Name)
				for _, a := range c.Attrs {
					fmt.Fprintf(&b, "      %s\n", a)
				}
			}
			fmt.Fprintf(&b, "  = %d unchanged\n\n", f.Same)
		}
	}

	if len(d.Errors) > 0 {
		b.WriteString("Errors:\n")
		for _, e := range d.Errors {
			fmt.Fprintf(&b, "  %s: %s\n", e.Section, e.Message)
		}
		b.WriteString("\n")
	}
	b.WriteString("Objects and fields only. Validation rules, flows, permission sets and layouts are not compared.\n")
	return b.String()
}

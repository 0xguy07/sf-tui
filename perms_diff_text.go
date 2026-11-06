package main

import (
	"fmt"
	"strings"

	"github.com/0xguy07/sf-tui/sf"
)

// renderPermissionsDiffText renders a PermissionsDiff as plain text, no ANSI.
func renderPermissionsDiffText(d sf.PermissionsDiff) string {
	var b strings.Builder
	b.WriteString("Permissions diff\n")
	fmt.Fprintf(&b, "  A: %s  (%s)\n", d.A.Name, d.A.Username)
	fmt.Fprintf(&b, "  B: %s  (%s)\n\n", d.B.Name, d.B.Username)

	list := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		b.WriteString(title + "\n")
		for _, it := range items {
			b.WriteString("  " + it + "\n")
		}
		b.WriteString("\n")
	}
	list("Assignments only A:", d.Assignments.OnlyA)
	list("Assignments only B:", d.Assignments.OnlyB)

	if len(d.Overrides) > 0 {
		b.WriteString("System overrides:\n")
		for _, o := range d.Overrides {
			name := map[string]string{"modifyAllData": "Modify All Data", "viewAllData": "View All Data"}[o.Name]
			fmt.Fprintf(&b, "  %-17s A[%s]  B[%s]", name, src(o.A), src(o.B))
			if o.SameGrant {
				b.WriteString("  (same grant, different source)")
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	notCompared := func(section string) string {
		for _, s := range d.NotCompared {
			if s != section {
				continue
			}
			var why []string
			for _, e := range d.Errors {
				if strings.HasSuffix(e.Section, ":"+section) {
					why = append(why, e.Section+": "+e.Message)
				}
			}
			return strings.Join(why, "; ")
		}
		return ""
	}
	rows := func(title, section string, gs []sf.GrantDiff) {
		if why := notCompared(section); why != "" {
			fmt.Fprintf(&b, "%s: not compared (%s)\n\n", title, why)
			return
		}
		if len(gs) == 0 {
			fmt.Fprintf(&b, "%s: no differences\n\n", title)
			return
		}
		plural := "differences"
		if len(gs) == 1 {
			plural = "difference"
		}
		fmt.Fprintf(&b, "%s (%d %s):\n", title, len(gs), plural)
		for _, g := range gs {
			fmt.Fprintf(&b, "  %-30s  %-9s  A[%s]  B[%s]", g.Name, g.Perm, src(g.A), src(g.B))
			if g.SameGrant {
				b.WriteString("  (same grant, different source)")
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	rows("Object permissions", "objects", d.Objects)
	if d.Object != "" {
		rows("Fields on "+d.Object, "fields", d.Fields)
	}

	if len(d.Errors) > 0 {
		b.WriteString("Errors:\n")
		for _, e := range d.Errors {
			fmt.Fprintf(&b, "  %s: %s\n", e.Section, e.Message)
		}
		b.WriteString("\n")
	}
	b.WriteString("Object and field permissions only. Sharing rules, roles and record ownership are not evaluated.\n")
	return b.String()
}

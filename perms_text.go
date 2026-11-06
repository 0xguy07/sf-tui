package main

import (
	"fmt"
	"strings"

	"github.com/0xguy07/sf-tui/sf"
)

// renderPermissionsText renders a PermissionsReport as plain text with no
// ANSI escapes, for `sftui perms --format text`.
func renderPermissionsText(r sf.PermissionsReport) string {
	var b strings.Builder

	head := fmt.Sprintf("%s  (%s)", r.User.Name, r.User.Username)
	if !r.User.IsActive {
		head += "  [inactive]"
	}
	fmt.Fprintln(&b, head)
	profile := r.User.ProfileName
	if profile == "" {
		profile = "(none)"
	}
	fmt.Fprintf(&b, "Profile: %s\n\n", profile)

	fmt.Fprintf(&b, "Assignments (%d):\n", len(r.Assignments))
	for _, a := range r.Assignments {
		line := "  " + assignmentLabel(a)
		if a.GroupLabel != "" {
			line += "  via group " + a.GroupLabel
		}
		fmt.Fprintln(&b, line)
	}
	b.WriteString("\n")

	if len(r.Overrides.ModifyAllData) > 0 || len(r.Overrides.ViewAllData) > 0 {
		b.WriteString("System overrides:\n")
		if len(r.Overrides.ModifyAllData) > 0 {
			fmt.Fprintf(&b, "  Modify All Data: %s  (implies edit/delete/view on every object)\n", strings.Join(r.Overrides.ModifyAllData, ", "))
		}
		if len(r.Overrides.ViewAllData) > 0 {
			fmt.Fprintf(&b, "  View All Data: %s\n", strings.Join(r.Overrides.ViewAllData, ", "))
		}
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "Object permissions (%d objects):\n", len(r.Objects))
	for _, o := range r.Objects {
		fmt.Fprintf(&b, "  %-30s  R[%s] C[%s] E[%s] D[%s] VA[%s] MA[%s]\n", o.SObject,
			src(o.Grants[sf.PermRead]), src(o.Grants[sf.PermCreate]), src(o.Grants[sf.PermEdit]),
			src(o.Grants[sf.PermDelete]), src(o.Grants[sf.PermViewAll]), src(o.Grants[sf.PermModifyAll]))
	}
	b.WriteString("\n")

	if r.Object != "" {
		fmt.Fprintf(&b, "Fields on %s (%d):\n", r.Object, len(r.Fields))
		if len(r.Fields) == 0 {
			b.WriteString("  (no FieldPermissions rows; fields follow profile defaults)\n")
			if !hasObject(r.Objects, r.Object) {
				fmt.Fprintf(&b, "  (no object permissions on %s either; check the API name)\n", r.Object)
			}
		}
		for _, f := range r.Fields {
			fmt.Fprintf(&b, "  %-30s  R[%s] E[%s]\n", f.Field, src(f.Grants[sf.PermRead]), src(f.Grants[sf.PermEdit]))
		}
		b.WriteString("\n")
	}

	if len(r.Errors) > 0 {
		b.WriteString("Errors:\n")
		for _, e := range r.Errors {
			fmt.Fprintf(&b, "  %s: %s\n", e.Section, e.Message)
		}
		b.WriteString("\n")
	}

	b.WriteString("Object and field permissions only. Sharing rules, roles and record ownership are not evaluated.\n")
	return b.String()
}

func assignmentLabel(a sf.PermSetAssignment) string {
	if a.IsProfile && a.ProfileName != "" {
		return "Profile: " + a.ProfileName
	}
	return a.PermSetLabel
}

// src joins a permission's sources with "+", or "-" when not granted.
func src(sources []string) string {
	if len(sources) == 0 {
		return "-"
	}
	return strings.Join(sources, "+")
}

func hasObject(objects []sf.ObjectPerm, name string) bool {
	for _, o := range objects {
		if strings.EqualFold(o.SObject, name) {
			return true
		}
	}
	return false
}

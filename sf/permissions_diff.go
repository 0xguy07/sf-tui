package sf

import (
	"reflect"
	"sort"
)

// PermissionsDiff compares two users' effective permissions in one org.
type PermissionsDiff struct {
	A           UserBrief       `json:"a"`
	B           UserBrief       `json:"b"`
	Assignments AssignmentsDiff `json:"assignments"`
	Overrides   []OverrideDiff  `json:"overrides"`
	Objects     []GrantDiff     `json:"objects"`
	Fields      []GrantDiff     `json:"fields"`
	Object      string          `json:"object,omitempty"`
	Different   bool            `json:"different"`
	Errors      []SectionError  `json:"errors"`
}

// AssignmentsDiff lists source labels assigned to one user only.
type AssignmentsDiff struct {
	OnlyA []string `json:"onlyA"`
	OnlyB []string `json:"onlyB"`
}

// OverrideDiff is a system permission whose sources differ between the users.
type OverrideDiff struct {
	Name string   `json:"name"` // modifyAllData | viewAllData
	A    []string `json:"a"`
	B    []string `json:"b"`
}

// GrantDiff is one (name, permission) where the users differ in grant or in
// the sources that grant it. SameGrant marks "both have it, for different reasons".
type GrantDiff struct {
	Name      string   `json:"name"`
	Perm      string   `json:"perm"`
	A         []string `json:"a"`
	B         []string `json:"b"`
	SameGrant bool     `json:"sameGrant,omitempty"`
}

var permOrder = map[string]int{PermRead: 0, PermCreate: 1, PermEdit: 2, PermDelete: 3, PermViewAll: 4, PermModifyAll: 5}

// DiffPermissions diffs two reports built for the same org (and the same
// --object, if any). Identical grants produce no rows.
func DiffPermissions(a, b PermissionsReport) PermissionsDiff {
	d := PermissionsDiff{
		A: a.User, B: b.User,
		Assignments: AssignmentsDiff{OnlyA: []string{}, OnlyB: []string{}},
		Overrides:   []OverrideDiff{},
		Objects:     []GrantDiff{},
		Fields:      []GrantDiff{},
		Object:      a.Object,
		Errors:      []SectionError{},
	}
	if d.Object == "" {
		d.Object = b.Object
	}

	// Assignments, by source label.
	labelSet := func(as []PermSetAssignment) map[string]bool {
		m := map[string]bool{}
		for _, x := range as {
			m[sourceLabel(x)] = true
		}
		return m
	}
	la, lb := labelSet(a.Assignments), labelSet(b.Assignments)
	for _, x := range a.Assignments {
		if l := sourceLabel(x); !lb[l] && !contains(d.Assignments.OnlyA, l) {
			d.Assignments.OnlyA = append(d.Assignments.OnlyA, l)
		}
	}
	for _, x := range b.Assignments {
		if l := sourceLabel(x); !la[l] && !contains(d.Assignments.OnlyB, l) {
			d.Assignments.OnlyB = append(d.Assignments.OnlyB, l)
		}
	}

	// Overrides.
	for _, o := range []struct {
		name string
		a, b []string
	}{{"modifyAllData", a.Overrides.ModifyAllData, b.Overrides.ModifyAllData}, {"viewAllData", a.Overrides.ViewAllData, b.Overrides.ViewAllData}} {
		if !sameStrings(o.a, o.b) {
			d.Overrides = append(d.Overrides, OverrideDiff{Name: o.name, A: nonNil(o.a), B: nonNil(o.b)})
		}
	}

	// Objects and fields.
	d.Objects = diffGrants(objectGrants(a.Objects), objectGrants(b.Objects))
	d.Fields = diffGrants(fieldGrants(a.Fields), fieldGrants(b.Fields))

	d.Different = len(d.Assignments.OnlyA)+len(d.Assignments.OnlyB)+len(d.Overrides)+len(d.Objects)+len(d.Fields) > 0

	for _, e := range a.Errors {
		d.Errors = append(d.Errors, SectionError{Section: "a:" + e.Section, Message: e.Message})
	}
	for _, e := range b.Errors {
		d.Errors = append(d.Errors, SectionError{Section: "b:" + e.Section, Message: e.Message})
	}
	return d
}

// grants is name → perm → sources.
type grants map[string]map[string][]string

func objectGrants(ops []ObjectPerm) grants {
	g := grants{}
	for _, o := range ops {
		g[o.SObject] = o.Grants
	}
	return g
}

func fieldGrants(fps []FieldPerm) grants {
	g := grants{}
	for _, f := range fps {
		g[f.Field] = f.Grants
	}
	return g
}

// diffGrants emits one row per (name, perm) whose source lists differ.
func diffGrants(ga, gb grants) []GrantDiff {
	names := map[string]bool{}
	for n := range ga {
		names[n] = true
	}
	for n := range gb {
		names[n] = true
	}
	var out []GrantDiff
	for n := range names {
		perms := map[string]bool{}
		for p := range ga[n] {
			perms[p] = true
		}
		for p := range gb[n] {
			perms[p] = true
		}
		for p := range perms {
			sa, sb := nonNil(ga[n][p]), nonNil(gb[n][p])
			if sameStrings(sa, sb) {
				continue
			}
			out = append(out, GrantDiff{Name: n, Perm: p, A: sa, B: sb, SameGrant: len(sa) > 0 && len(sb) > 0})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return permOrder[out[i].Perm] < permOrder[out[j].Perm]
	})
	if out == nil {
		out = []GrantDiff{}
	}
	return out
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func sameStrings(a, b []string) bool {
	return reflect.DeepEqual(nonNil(a), nonNil(b))
}

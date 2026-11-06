package sf

import (
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
	// NotCompared lists sections ("objects", "fields") skipped because one
	// side failed to load them; a failure on one side invalidates the diff.
	NotCompared []string       `json:"notCompared"`
	Different   bool           `json:"different"`
	Errors      []SectionError `json:"errors"`
}

// AssignmentsDiff lists source labels assigned to one user only (by Id).
type AssignmentsDiff struct {
	OnlyA []string `json:"onlyA"`
	OnlyB []string `json:"onlyB"`
}

// OverrideDiff is a system permission whose sources differ between the users.
type OverrideDiff struct {
	Name      string   `json:"name"` // modifyAllData | viewAllData
	A         []string `json:"a"`
	B         []string `json:"b"`
	SameGrant bool     `json:"sameGrant"` // both have it, from different sources
}

// GrantDiff is one (name, permission) where the users differ in grant or in
// the sources that grant it. SameGrant marks "both have it, for different reasons".
type GrantDiff struct {
	Name      string   `json:"name"`
	Perm      string   `json:"perm"`
	A         []string `json:"a"`
	B         []string `json:"b"`
	SameGrant bool     `json:"sameGrant"`
}

var permOrder = map[string]int{PermRead: 0, PermCreate: 1, PermEdit: 2, PermDelete: 3, PermViewAll: 4, PermModifyAll: 5}

// DiffPermissions diffs two reports built for the same org (and the same
// --object, if any). Source lists are compared as sets, since SOQL does not
// guarantee their order, and every list in the result is sorted.
func DiffPermissions(a, b PermissionsReport) PermissionsDiff {
	d := PermissionsDiff{
		A: a.User, B: b.User,
		Assignments: AssignmentsDiff{OnlyA: []string{}, OnlyB: []string{}},
		Overrides:   []OverrideDiff{},
		Objects:     []GrantDiff{},
		Fields:      []GrantDiff{},
		Object:      a.Object,
		NotCompared: []string{},
		Errors:      []SectionError{},
	}
	if d.Object == "" {
		d.Object = b.Object
	}

	// Assignments by permission set Id; labels are for display only.
	ids := func(as []PermSetAssignment) map[string]bool {
		m := map[string]bool{}
		for _, x := range as {
			m[x.PermSetID] = true
		}
		return m
	}
	ia, ib := ids(a.Assignments), ids(b.Assignments)
	for _, x := range a.Assignments {
		if !ib[x.PermSetID] {
			d.Assignments.OnlyA = append(d.Assignments.OnlyA, sourceLabel(x))
		}
	}
	for _, x := range b.Assignments {
		if !ia[x.PermSetID] {
			d.Assignments.OnlyB = append(d.Assignments.OnlyB, sourceLabel(x))
		}
	}
	sort.Strings(d.Assignments.OnlyA)
	sort.Strings(d.Assignments.OnlyB)

	// Overrides.
	for _, o := range []struct {
		name string
		a, b []string
	}{{"modifyAllData", a.Overrides.ModifyAllData, b.Overrides.ModifyAllData}, {"viewAllData", a.Overrides.ViewAllData, b.Overrides.ViewAllData}} {
		sa, sb := sortedSet(o.a), sortedSet(o.b)
		if !equalStrings(sa, sb) {
			d.Overrides = append(d.Overrides, OverrideDiff{Name: o.name, A: sa, B: sb, SameGrant: len(sa) > 0 && len(sb) > 0})
		}
	}

	// Objects and fields: skipped when either side failed to load them.
	failed := map[string]bool{}
	for _, e := range a.Errors {
		failed[e.Section] = true
	}
	for _, e := range b.Errors {
		failed[e.Section] = true
	}
	if failed["objects"] {
		d.NotCompared = append(d.NotCompared, "objects")
	} else {
		d.Objects = diffGrants(objectGrants(a.Objects), objectGrants(b.Objects))
	}
	if failed["fields"] {
		d.NotCompared = append(d.NotCompared, "fields")
	} else {
		d.Fields = diffGrants(fieldGrants(a.Fields), fieldGrants(b.Fields))
	}

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

// diffGrants emits one row per (name, perm) whose source sets differ.
func diffGrants(ga, gb grants) []GrantDiff {
	names := map[string]bool{}
	for n := range ga {
		names[n] = true
	}
	for n := range gb {
		names[n] = true
	}
	out := []GrantDiff{}
	for n := range names {
		perms := map[string]bool{}
		for p := range ga[n] {
			perms[p] = true
		}
		for p := range gb[n] {
			perms[p] = true
		}
		for p := range perms {
			sa, sb := sortedSet(ga[n][p]), sortedSet(gb[n][p])
			if equalStrings(sa, sb) {
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
	return out
}

// sortedSet returns a sorted, deduplicated, non-nil copy.
func sortedSet(s []string) []string {
	out := make([]string, 0, len(s))
	for _, v := range s {
		if !contains(out, v) {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

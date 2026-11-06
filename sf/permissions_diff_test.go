package sf

import (
	"encoding/json"
	"reflect"
	"testing"
)

func permsReport(t *testing.T, name string) PermissionsReport {
	t.Helper()
	var r PermissionsReport
	if err := json.Unmarshal(permsFixture(t, name), &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestDiffPermissions_Assignments(t *testing.T) {
	d := DiffPermissions(permsReport(t, "report_account.json"), permsReport(t, "report_bob.json"))
	if !reflect.DeepEqual(d.Assignments.OnlyA, []string{"Data Steward", "Sales Ops"}) || len(d.Assignments.OnlyB) != 0 || d.Assignments.OnlyB == nil {
		t.Errorf("assignments = %+v", d.Assignments)
	}
	if d.A.Name != "Alex Admin" || d.B.Name != "Bob Builder" || !d.Different {
		t.Errorf("users/different wrong: %+v %+v %v", d.A, d.B, d.Different)
	}
}

func TestDiffPermissions_Overrides(t *testing.T) {
	d := DiffPermissions(permsReport(t, "report_account.json"), permsReport(t, "report_bob.json"))
	want := []OverrideDiff{{Name: "modifyAllData", A: []string{"Data Steward"}, B: []string{}}}
	if !reflect.DeepEqual(d.Overrides, want) {
		t.Errorf("overrides = %+v, want %+v", d.Overrides, want)
	}
}

func TestDiffPermissions_ObjectsAndOrder(t *testing.T) {
	d := DiffPermissions(permsReport(t, "report_account.json"), permsReport(t, "report_bob.json"))
	want := []GrantDiff{
		{Name: "Account", Perm: PermCreate, A: []string{"Sales Ops"}, B: []string{}},
		{Name: "Account", Perm: PermEdit, A: []string{"Sales Ops"}, B: []string{}},
		{Name: "Case", Perm: PermRead, A: []string{"Reports Viewer", "Sales Ops"}, B: []string{}},
		{Name: "Case", Perm: PermEdit, A: []string{"Sales Ops"}, B: []string{}},
	}
	if !reflect.DeepEqual(d.Objects, want) {
		t.Errorf("objects =\n%+v\nwant\n%+v", d.Objects, want)
	}
}

func TestDiffPermissions_FieldsSameGrantDifferentSource(t *testing.T) {
	d := DiffPermissions(permsReport(t, "report_account.json"), permsReport(t, "report_bob.json"))
	want := []GrantDiff{
		{Name: "Industry", Perm: PermRead, A: []string{"Profile: Sales User", "Sales Ops"}, B: []string{"Profile: Sales User"}, SameGrant: true},
		{Name: "Industry", Perm: PermEdit, A: []string{"Sales Ops"}, B: []string{}},
		{Name: "Rating", Perm: PermRead, A: []string{"Sales Ops"}, B: []string{}},
	}
	if !reflect.DeepEqual(d.Fields, want) || d.Object != "Account" {
		t.Errorf("fields =\n%+v\nwant\n%+v (object %q)", d.Fields, want, d.Object)
	}
}

func TestDiffPermissions_Identical(t *testing.T) {
	a := permsReport(t, "report_account.json")
	d := DiffPermissions(a, a)
	if d.Different || len(d.Objects) != 0 || len(d.Fields) != 0 || len(d.Overrides) != 0 || len(d.Assignments.OnlyA) != 0 {
		t.Errorf("identical users should produce no rows: %+v", d)
	}
	if d.Objects == nil || d.Fields == nil || d.Overrides == nil || d.Errors == nil {
		t.Error("slices must be non-nil")
	}
}

func TestDiffPermissions_EmptySide(t *testing.T) {
	a := permsReport(t, "report_account.json")
	empty := PermissionsReport{User: UserBrief{Name: "Nobody"}, Assignments: []PermSetAssignment{}, Objects: []ObjectPerm{}, Errors: []SectionError{}}
	d := DiffPermissions(a, empty)
	if len(d.Objects) != 5 || !d.Different { // Account read/create/edit + Case read/edit
		t.Errorf("objects = %+v", d.Objects)
	}
	d = DiffPermissions(empty, a)
	if len(d.Objects) != 5 || len(d.Objects[0].A) != 0 || len(d.Objects[0].B) == 0 {
		t.Errorf("reversed: %+v", d.Objects)
	}
}

func TestDiffPermissions_ErrorsPrefixed(t *testing.T) {
	a := permsReport(t, "report_account.json")
	b := permsReport(t, "report_bob.json")
	a.Errors = []SectionError{{Section: "objects", Message: "boom"}}
	b.Errors = []SectionError{{Section: "fields", Message: "bang"}}
	d := DiffPermissions(a, b)
	want := []SectionError{{Section: "a:objects", Message: "boom"}, {Section: "b:fields", Message: "bang"}}
	if !reflect.DeepEqual(d.Errors, want) {
		t.Errorf("errors = %+v", d.Errors)
	}
}

func reorderSources(r PermissionsReport) PermissionsReport {
	rev := func(s []string) []string {
		out := make([]string, len(s))
		for i, v := range s {
			out[len(s)-1-i] = v
		}
		return out
	}
	for i := range r.Objects {
		for k, v := range r.Objects[i].Grants {
			r.Objects[i].Grants[k] = rev(v)
		}
	}
	for i := range r.Fields {
		for k, v := range r.Fields[i].Grants {
			r.Fields[i].Grants[k] = rev(v)
		}
	}
	r.Overrides.ModifyAllData = rev(r.Overrides.ModifyAllData)
	r.Overrides.ViewAllData = rev(r.Overrides.ViewAllData)
	for i, j := 0, len(r.Assignments)-1; i < j; i, j = i+1, j-1 {
		r.Assignments[i], r.Assignments[j] = r.Assignments[j], r.Assignments[i]
	}
	return r
}

func TestDiffPermissions_SourceOrderDoesNotMatter(t *testing.T) {
	a := permsReport(t, "report_account.json")
	b := reorderSources(permsReport(t, "report_account.json"))
	d := DiffPermissions(a, b)
	if d.Different || len(d.Objects)+len(d.Fields)+len(d.Overrides)+len(d.Assignments.OnlyA)+len(d.Assignments.OnlyB) != 0 {
		t.Errorf("identical access in a different SOQL order must produce no rows: %+v", d)
	}
}

func TestDiffPermissions_OutputSorted(t *testing.T) {
	d := DiffPermissions(permsReport(t, "report_account.json"), permsReport(t, "report_bob.json"))
	if !reflect.DeepEqual(d.Assignments.OnlyA, []string{"Data Steward", "Sales Ops"}) {
		t.Errorf("onlyA should be sorted: %v", d.Assignments.OnlyA)
	}
	for _, g := range d.Objects {
		if g.Name == "Case" && g.Perm == PermRead && !reflect.DeepEqual(g.A, []string{"Reports Viewer", "Sales Ops"}) {
			t.Errorf("sources should be sorted: %v", g.A)
		}
	}
}

func TestDiffPermissions_ErroredSectionSkipped(t *testing.T) {
	a := permsReport(t, "report_account.json")
	b := permsReport(t, "report_bob.json")
	b.Objects = []ObjectPerm{}
	b.Errors = []SectionError{{Section: "objects", Message: "timeout"}}
	d := DiffPermissions(a, b)
	if len(d.Objects) != 0 || !reflect.DeepEqual(d.NotCompared, []string{"objects"}) {
		t.Errorf("a failed section must not produce rows: objects=%+v notCompared=%v", d.Objects, d.NotCompared)
	}
	if len(d.Fields) == 0 {
		t.Error("the fields section had no error and should still be compared")
	}
}

func TestDiffPermissions_AssignmentsById(t *testing.T) {
	a := permsReport(t, "report_account.json")
	b := permsReport(t, "report_bob.json")
	// Give Bob a *different* permission set that happens to share Sales Ops's label.
	b.Assignments = append(b.Assignments, PermSetAssignment{PermSetID: "0PS000000000099AAA", PermSetLabel: "Sales Ops"})
	d := DiffPermissions(a, b)
	if !contains(d.Assignments.OnlyA, "Sales Ops") || !contains(d.Assignments.OnlyB, "Sales Ops") {
		t.Errorf("same label, different Id must count as only-A and only-B: %+v", d.Assignments)
	}
}

func TestDiffPermissions_OverrideSameGrant(t *testing.T) {
	a := permsReport(t, "report_account.json")
	b := permsReport(t, "report_bob.json")
	b.Overrides.ModifyAllData = []string{"Ops Admin"}
	d := DiffPermissions(a, b)
	if len(d.Overrides) != 1 || !d.Overrides[0].SameGrant {
		t.Errorf("both have Modify All Data from different sets → SameGrant: %+v", d.Overrides)
	}
}

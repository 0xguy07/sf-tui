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
	if !reflect.DeepEqual(d.Assignments.OnlyA, []string{"Sales Ops", "Data Steward"}) || len(d.Assignments.OnlyB) != 0 || d.Assignments.OnlyB == nil {
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
		{Name: "Case", Perm: PermRead, A: []string{"Sales Ops", "Reports Viewer"}, B: []string{}},
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

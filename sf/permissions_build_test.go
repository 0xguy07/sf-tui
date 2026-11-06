package sf

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func permsFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("../testdata/perms", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func permsRecords(t *testing.T, name string) []map[string]any {
	t.Helper()
	recs, err := decodeRecords(permsFixture(t, name))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return recs
}

func TestDecodeRecords(t *testing.T) {
	recs, err := decodeRecords(permsFixture(t, "assignments.json"))
	if err != nil || len(recs) != 4 {
		t.Fatalf("got %d records, err %v", len(recs), err)
	}
	if _, err := decodeRecords([]byte("not json")); err == nil {
		t.Error("garbage should error")
	}
}

func TestBuildAssignments(t *testing.T) {
	as, _ := buildAssignments(permsRecords(t, "assignments.json"))
	var labels []string
	for _, a := range as {
		labels = append(labels, sourceLabel(a))
	}
	want := []string{"Profile: Sales User", "Sales Ops", "Data Steward", "Reports Viewer"}
	if !reflect.DeepEqual(labels, want) {
		t.Errorf("labels = %v, want %v", labels, want)
	}
	if !as[0].IsProfile || as[1].IsProfile || as[2].IsProfile || as[3].IsProfile {
		t.Error("IsProfile should be true only for the first assignment")
	}
	if as[2].GroupLabel != "Ops Group" || as[0].GroupLabel != "" || as[1].GroupLabel != "" {
		t.Errorf("GroupLabel wrong: %+v", as)
	}
}

func TestBuildAssignments_Overrides(t *testing.T) {
	_, ov := buildAssignments(permsRecords(t, "assignments.json"))
	if !reflect.DeepEqual(ov.ModifyAllData, []string{"Data Steward"}) {
		t.Errorf("ModifyAllData = %v", ov.ModifyAllData)
	}
	if !reflect.DeepEqual(ov.ViewAllData, []string{"Reports Viewer"}) {
		t.Errorf("ViewAllData = %v", ov.ViewAllData)
	}
}

func TestLabelsOf(t *testing.T) {
	as, _ := buildAssignments(permsRecords(t, "assignments.json"))
	m := labelsOf(as)
	if len(m) != 4 || m["0PS000000000002AAA"] != "Sales Ops" || m["0PS000000000001AAA"] != "Profile: Sales User" {
		t.Errorf("labelsOf = %v", m)
	}
}

func testLabels(t *testing.T) map[string]string {
	t.Helper()
	as, _ := buildAssignments(permsRecords(t, "assignments.json"))
	return labelsOf(as)
}

func TestBuildObjectPerms_Grants(t *testing.T) {
	ops := buildObjectPerms(permsRecords(t, "objectperms.json"), testLabels(t))
	byName := map[string]ObjectPerm{}
	for _, o := range ops {
		byName[o.SObject] = o
	}
	acc := byName["Account"]
	if !reflect.DeepEqual(acc.Grants[PermRead], []string{"Profile: Sales User"}) ||
		!reflect.DeepEqual(acc.Grants[PermCreate], []string{"Sales Ops"}) ||
		!reflect.DeepEqual(acc.Grants[PermEdit], []string{"Sales Ops"}) ||
		!reflect.DeepEqual(acc.Grants[PermDelete], []string{}) {
		t.Errorf("Account grants = %v", acc.Grants)
	}
	if !reflect.DeepEqual(acc.Sources, []string{"Profile: Sales User", "Sales Ops"}) {
		t.Errorf("Account sources = %v", acc.Sources)
	}
	if !acc.Read || !acc.Create || !acc.Edit || acc.Delete {
		t.Errorf("Account booleans = %+v", acc)
	}
	if got := byName["Case"].Grants[PermRead]; !reflect.DeepEqual(got, []string{"Sales Ops", "Reports Viewer"}) {
		t.Errorf("Case read grants = %v", got)
	}
}

func TestBuildObjectPerms_NoGrantRow(t *testing.T) {
	ops := buildObjectPerms(permsRecords(t, "objectperms.json"), testLabels(t))
	var lead *ObjectPerm
	for i := range ops {
		if ops[i].SObject == "Lead" {
			lead = &ops[i]
		}
	}
	if lead == nil {
		t.Fatal("Lead missing")
	}
	if len(lead.Sources) != 0 {
		t.Errorf("Lead sources = %v, want none", lead.Sources)
	}
	for _, k := range []string{PermRead, PermCreate, PermEdit, PermDelete, PermViewAll, PermModifyAll} {
		if v, ok := lead.Grants[k]; !ok || v == nil || len(v) != 0 {
			t.Errorf("Lead Grants[%s] = %v (present=%v), want empty non-nil", k, v, ok)
		}
	}
}

func TestBuildObjectPerms_Sorted(t *testing.T) {
	ops := buildObjectPerms(permsRecords(t, "objectperms.json"), testLabels(t))
	var names []string
	for _, o := range ops {
		names = append(names, o.SObject)
	}
	if !reflect.DeepEqual(names, []string{"Account", "Case", "Lead"}) {
		t.Errorf("order = %v", names)
	}
}

func TestBuildFieldPerms(t *testing.T) {
	fps := buildFieldPerms(permsRecords(t, "fieldperms.json"), testLabels(t))
	if len(fps) != 2 || fps[0].Field != "Industry" || fps[1].Field != "Rating" {
		t.Fatalf("fields = %+v", fps)
	}
	if !reflect.DeepEqual(fps[0].Grants[PermRead], []string{"Profile: Sales User", "Sales Ops"}) ||
		!reflect.DeepEqual(fps[0].Grants[PermEdit], []string{"Sales Ops"}) {
		t.Errorf("Industry grants = %v", fps[0].Grants)
	}
	if !reflect.DeepEqual(fps[1].Grants[PermEdit], []string{}) || !fps[1].Read || fps[1].Edit {
		t.Errorf("Rating = %+v", fps[1])
	}
}

func TestBuildFieldPerms_Empty(t *testing.T) {
	fps := buildFieldPerms(permsRecords(t, "fieldperms_empty.json"), testLabels(t))
	if fps == nil || len(fps) != 0 {
		t.Errorf("want empty non-nil slice, got %#v", fps)
	}
}

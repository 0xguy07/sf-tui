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

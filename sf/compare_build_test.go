package sf

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func compareFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("../testdata/compare", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func fixtureSObjects(t *testing.T, name string) []SObjectSummary {
	t.Helper()
	var r sobjectListResult
	if err := json.Unmarshal(compareFixture(t, name), &r); err != nil {
		t.Fatal(err)
	}
	return r.Result.SObjects
}

func fixtureDescribe(t *testing.T, name string) *Describe {
	t.Helper()
	var r describeResult
	if err := json.Unmarshal(compareFixture(t, name), &r); err != nil {
		t.Fatal(err)
	}
	return &r.Result
}

func TestDiffSObjects(t *testing.T) {
	states, sum := DiffSObjects(fixtureSObjects(t, "sobjects_a.json"), fixtureSObjects(t, "sobjects_b.json"))
	want := []ObjectState{
		{Name: "Account", State: StateBoth},
		{Name: "Contact", State: StateBoth},
		{Name: "Legacy__c", Custom: true, State: StateOnlyA},
		{Name: "NewThing__c", Custom: true, State: StateOnlyB},
		{Name: "Widget__c", Custom: true, State: StateBoth},
	}
	if !reflect.DeepEqual(states, want) {
		t.Errorf("states = %+v", states)
	}
	if sum.OnlyA != 1 || sum.OnlyB != 1 || sum.Both != 3 {
		t.Errorf("summary = %+v", sum)
	}
}

func TestDiffFields_Identical(t *testing.T) {
	d := DiffFields("Account", fixtureDescribe(t, "describe_account_a.json"), fixtureDescribe(t, "describe_account_b.json"))
	if d.Different || d.Same != 3 || d.OnlyA == nil || d.OnlyB == nil || d.Changed == nil || len(d.OnlyA)+len(d.OnlyB)+len(d.Changed) != 0 {
		t.Errorf("identical diff = %+v", d)
	}
}

func TestDiffFields_Widget(t *testing.T) {
	d := DiffFields("Widget__c", fixtureDescribe(t, "describe_widget_a.json"), fixtureDescribe(t, "describe_widget_b.json"))
	if !d.Different || d.SObject != "Widget__c" {
		t.Fatalf("diff = %+v", d)
	}
	if !reflect.DeepEqual(d.OnlyA, []FieldBrief{{Name: "Old__c", Type: "string(40) [custom]"}}) {
		t.Errorf("OnlyA = %+v", d.OnlyA)
	}
	if !reflect.DeepEqual(d.OnlyB, []FieldBrief{{Name: "New__c", Type: "double [custom]"}}) {
		t.Errorf("OnlyB = %+v", d.OnlyB)
	}
	if len(d.Changed) != 2 || d.Changed[0].Name != "Name" || d.Changed[1].Name != "Status__c" {
		t.Fatalf("Changed = %+v", d.Changed)
	}
	if !reflect.DeepEqual(d.Changed[0].Attrs, []string{"length: 80 → 255"}) || !reflect.DeepEqual(d.Changed[1].Attrs, []string{"picklist values differ"}) {
		t.Errorf("attrs = %+v / %+v", d.Changed[0].Attrs, d.Changed[1].Attrs)
	}
	if d.Same != 2 {
		t.Errorf("Same = %d, want 2 (Id, OwnerId)", d.Same)
	}
}

func TestDiffFields_OneSideNil(t *testing.T) {
	d := DiffFields("Widget__c", fixtureDescribe(t, "describe_widget_a.json"), nil)
	if !d.Different || len(d.OnlyA) != 5 || len(d.OnlyB) != 0 || len(d.Changed) != 0 || d.Same != 0 {
		t.Errorf("diff = %+v", d)
	}
}

func TestIsCustomName(t *testing.T) {
	for name, want := range map[string]bool{"Widget__c": true, "X__mdt": true, "E__e": true, "Z__x": true, "B__b": true, "Account": false, "Foo__History": false, "Case": false} {
		if IsCustomName(name) != want {
			t.Errorf("IsCustomName(%s) != %v", name, want)
		}
	}
}

func TestSameRefsAndPicklist(t *testing.T) {
	if !sameRefs(nil, nil) || !sameRefs([]string{"a", "b"}, []string{"b", "a"}) || sameRefs([]string{"a"}, []string{"b"}) {
		t.Error("sameRefs")
	}
	a := []string{"b", "a"}
	sameRefs(a, []string{"a", "b"})
	if a[0] != "b" {
		t.Error("sameRefs must not reorder its inputs")
	}
	pv := func(v string, active bool) PicklistValue { return PicklistValue{Value: v, Active: active} }
	if !samePicklist([]PicklistValue{pv("a", true), pv("b", true)}, []PicklistValue{pv("b", true), pv("a", true)}) {
		t.Error("order should not matter")
	}
	if samePicklist([]PicklistValue{pv("a", true)}, []PicklistValue{pv("b", true)}) {
		t.Error("different values should differ")
	}
}

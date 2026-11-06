package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/0xguy07/sf-tui/sf"
)

func fixtureDiff(t *testing.T) sf.SchemaDiff {
	t.Helper()
	b, err := os.ReadFile("testdata/compare/diff_full.json")
	if err != nil {
		t.Fatal(err)
	}
	var d sf.SchemaDiff
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestRenderSchemaDiffText_Golden(t *testing.T) {
	const golden = "testdata/compare/compare_full.txt"
	got := renderSchemaDiffText(fixtureDiff(t))
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("missing golden (run with UPDATE_GOLDEN=1): %v", err)
	}
	if got != string(want) {
		t.Errorf("differs from golden\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRenderSchemaDiffText_NoDifferences(t *testing.T) {
	d := sf.SchemaDiff{
		A: sf.Side{Org: "a", SObjects: 3}, B: sf.Side{Org: "b", SObjects: 3},
		Objects: []sf.ObjectState{{Name: "Account", State: sf.StateBoth}, {Name: "Case", State: sf.StateBoth}, {Name: "Lead", State: sf.StateBoth}},
		Fields:  []sf.ObjectFieldDiff{}, Summary: sf.Summary{Both: 3}, Errors: []sf.SectionError{},
	}
	out := renderSchemaDiffText(d)
	if !strings.Contains(out, "Objects: 0 only in A, 0 only in B, 3 in both") || strings.Contains(out, "Only in A:") || strings.Contains(out, "Fields:") {
		t.Errorf("no-difference output wrong:\n%s", out)
	}
}

func TestRenderSchemaDiffText_ErrorsSection(t *testing.T) {
	d := fixtureDiff(t)
	d.Errors = []sf.SectionError{{Section: "describe-b:Contact", Message: "boom"}}
	if out := renderSchemaDiffText(d); !strings.Contains(out, "Errors:\n  describe-b:Contact: boom") {
		t.Errorf("errors section missing:\n%s", out)
	}
	if out := renderSchemaDiffText(fixtureDiff(t)); strings.Contains(out, "Errors:") {
		t.Error("errors section should be omitted when none")
	}
}

func TestRenderSchemaDiffText_NoANSI(t *testing.T) {
	if strings.Contains(renderSchemaDiffText(fixtureDiff(t)), "\x1b") {
		t.Error("ANSI escapes in output")
	}
}

func TestRenderSchemaDiffText_FieldLevelZeroObjects(t *testing.T) {
	d := fixtureDiff(t)
	d.Fields, d.FieldLevel = []sf.ObjectFieldDiff{}, true
	d.Summary.ObjectsCompared, d.Summary.ObjectsChanged = 0, 0
	if out := renderSchemaDiffText(d); !strings.Contains(out, "Fields: 0 objects compared, 0 with differences") {
		t.Errorf("Fields line missing when field level ran on zero objects:\n%s", out)
	}
}

func TestRenderSchemaDiffText_ObjectModeIdentical(t *testing.T) {
	ident := sf.ObjectFieldDiff{SObject: "Account", OnlyA: []sf.FieldBrief{}, OnlyB: []sf.FieldBrief{}, Changed: []sf.FieldChange{}, Same: 3}
	d := fixtureDiff(t)
	d.FieldLevel, d.Object, d.Fields = true, "Account", []sf.ObjectFieldDiff{ident}
	d.Summary.ObjectsCompared, d.Summary.ObjectsChanged = 1, 0
	if out := renderSchemaDiffText(d); !strings.Contains(out, "Account: no field-level differences (3 fields)") {
		t.Errorf("--object identical line missing:\n%s", out)
	}
	// Under --fields (no Object), identical objects are omitted even if only one was compared.
	d.Object = ""
	if out := renderSchemaDiffText(d); strings.Contains(out, "no field-level differences") {
		t.Errorf("--fields mode should omit identical objects:\n%s", out)
	}
}

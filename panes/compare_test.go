package panes

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/0xguy07/sf-tui/sf"
)

func cmpSObjects(t *testing.T, name string) []sf.SObjectSummary {
	t.Helper()
	b, err := os.ReadFile("../testdata/compare/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var r struct {
		Result struct {
			SObjects []sf.SObjectSummary `json:"sobjects"`
		} `json:"result"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return r.Result.SObjects
}

func cmpDescribe(t *testing.T, name string) *sf.Describe {
	t.Helper()
	b, err := os.ReadFile("../testdata/compare/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var r struct {
		Result sf.Describe `json:"result"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return &r.Result
}

func TestCompareRefresh_Counts(t *testing.T) {
	c := NewCompare(120, 40)
	c.SetSchema("A", "aorg", cmpSObjects(t, "sobjects_a.json"), false)
	c.SetSchema("B", "borg", cmpSObjects(t, "sobjects_b.json"), false)
	out := c.Viewport.View()
	for _, want := range []string{"only in A:  1", "only in B:  1", "in both:    3"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q:\n%s", want, out)
		}
	}
	if n := len(c.List.Items()); n != 5 {
		t.Errorf("list items = %d, want 5", n)
	}
}

func TestCompareRefresh_CachedMarker(t *testing.T) {
	c := NewCompare(120, 40)
	c.SetSchema("A", "aorg", cmpSObjects(t, "sobjects_a.json"), true)
	c.SetSchema("B", "borg", cmpSObjects(t, "sobjects_b.json"), false)
	if out := c.Viewport.View(); !strings.Contains(out, "(cached)") {
		t.Errorf("cached marker missing:\n%s", out)
	}
}

func TestRenderFieldDiff_Widget(t *testing.T) {
	msg := sf.FieldDiffLoadedMsg{SObject: "Widget__c", A: cmpDescribe(t, "describe_widget_a.json"), B: cmpDescribe(t, "describe_widget_b.json")}
	out := renderFieldDiff(msg, "aorg", "borg")
	for _, want := range []string{"- Old__c", "+ New__c", "~ Name", "length: 80 → 255"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
}

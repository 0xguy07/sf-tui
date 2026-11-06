package sf

import (
	"fmt"
	"sort"
	"strings"
)

// Object states in a schema compare.
const (
	StateOnlyA = "only-a"
	StateOnlyB = "only-b"
	StateBoth  = "both"
)

// Side describes one org in a compare.
type Side struct {
	Org      string `json:"org"`
	SObjects int    `json:"sobjects"`
	Cached   bool   `json:"cached"`
}

// ObjectState is one sobject's presence across the two sides.
type ObjectState struct {
	Name   string `json:"name"`
	Custom bool   `json:"custom"`
	State  string `json:"state"`
}

// FieldBrief names a field present on one side only.
type FieldBrief struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// FieldChange is a field present on both sides with differing attributes.
type FieldChange struct {
	Name  string   `json:"name"`
	Attrs []string `json:"attrs"`
}

// ObjectFieldDiff is the field-level diff of one sobject.
type ObjectFieldDiff struct {
	SObject   string        `json:"sobject"`
	OnlyA     []FieldBrief  `json:"onlyA"`
	OnlyB     []FieldBrief  `json:"onlyB"`
	Changed   []FieldChange `json:"changed"`
	Same      int           `json:"same"`
	Different bool          `json:"different"`
}

// Summary carries the counts shown in headers.
type Summary struct {
	OnlyA           int `json:"onlyA"`
	OnlyB           int `json:"onlyB"`
	Both            int `json:"both"`
	ObjectsCompared int `json:"objectsCompared"`
	ObjectsChanged  int `json:"objectsChanged"`
}

// SchemaDiff is the full result of comparing two orgs.
type SchemaDiff struct {
	A         Side              `json:"a"`
	B         Side              `json:"b"`
	Objects   []ObjectState     `json:"objects"`
	Fields    []ObjectFieldDiff `json:"fields"`
	Summary   Summary           `json:"summary"`
	Different bool              `json:"different"`
	Errors    []SectionError    `json:"errors"`
}

// DiffSObjects classifies the union of two sobject lists, sorted by name.
func DiffSObjects(a, b []SObjectSummary) ([]ObjectState, Summary) {
	aBy := map[string]SObjectSummary{}
	bBy := map[string]SObjectSummary{}
	for _, s := range a {
		aBy[s.Name] = s
	}
	for _, s := range b {
		bBy[s.Name] = s
	}
	names := map[string]bool{}
	for n := range aBy {
		names[n] = true
	}
	for n := range bBy {
		names[n] = true
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)

	out := make([]ObjectState, 0, len(sorted))
	var sum Summary
	for _, n := range sorted {
		as, inA := aBy[n]
		bs, inB := bBy[n]
		st := ObjectState{Name: n}
		switch {
		case inA && !inB:
			st.State, st.Custom = StateOnlyA, as.Custom
			sum.OnlyA++
		case !inA && inB:
			st.State, st.Custom = StateOnlyB, bs.Custom
			sum.OnlyB++
		default:
			st.State, st.Custom = StateBoth, as.Custom || bs.Custom
			sum.Both++
		}
		out = append(out, st)
	}
	return out, sum
}

// DiffFields compares the fields of one sobject across two describes. A nil
// side means the object is absent there: every field of the other side is
// reported as only-that-side.
func DiffFields(sobject string, a, b *Describe) ObjectFieldDiff {
	d := ObjectFieldDiff{SObject: sobject, OnlyA: []FieldBrief{}, OnlyB: []FieldBrief{}, Changed: []FieldChange{}}
	aFields := map[string]Field{}
	bFields := map[string]Field{}
	if a != nil {
		for _, f := range a.Fields {
			aFields[f.Name] = f
		}
	}
	if b != nil {
		for _, f := range b.Fields {
			bFields[f.Name] = f
		}
	}
	all := map[string]bool{}
	for n := range aFields {
		all[n] = true
	}
	for n := range bFields {
		all[n] = true
	}
	names := make([]string, 0, len(all))
	for n := range all {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		af, inA := aFields[n]
		bf, inB := bFields[n]
		switch {
		case inA && !inB:
			d.OnlyA = append(d.OnlyA, FieldBrief{Name: n, Type: FieldTypeLabel(af)})
		case !inA && inB:
			d.OnlyB = append(d.OnlyB, FieldBrief{Name: n, Type: FieldTypeLabel(bf)})
		default:
			if attrs := FieldAttrDiff(af, bf); len(attrs) == 0 {
				d.Same++
			} else {
				d.Changed = append(d.Changed, FieldChange{Name: n, Attrs: attrs})
			}
		}
	}
	d.Different = len(d.OnlyA)+len(d.OnlyB)+len(d.Changed) > 0
	return d
}

// IsCustomName reports whether an sobject API name has a custom suffix.
func IsCustomName(name string) bool {
	for _, suf := range []string{"__c", "__mdt", "__e", "__x", "__b", "__xo", "__kav"} {
		if strings.HasSuffix(name, suf) {
			return true
		}
	}
	return false
}

// FieldTypeLabel is the compact type description used in diffs, e.g.
// "string(80) [required,custom]".
func FieldTypeLabel(f Field) string {
	t := f.Type
	if f.Type == "reference" && len(f.ReferenceTo) > 0 {
		t = "reference→" + strings.Join(f.ReferenceTo, ",")
	}
	if f.Length > 0 && (f.Type == "string" || f.Type == "textarea") {
		t = fmt.Sprintf("%s(%d)", f.Type, f.Length)
	}
	tags := []string{}
	if !f.Nillable {
		tags = append(tags, "required")
	}
	if f.Custom {
		tags = append(tags, "custom")
	}
	if !f.Updateable {
		tags = append(tags, "read-only")
	}
	if len(tags) > 0 {
		t += " [" + strings.Join(tags, ",") + "]"
	}
	return t
}

// FieldAttrDiff returns the human-readable list of attributes that differ
// between the two sides of the same field. Empty means identical.
func FieldAttrDiff(a, b Field) []string {
	var diffs []string
	if a.Type != b.Type {
		diffs = append(diffs, fmt.Sprintf("type: %s → %s", a.Type, b.Type))
	}
	if a.Length != b.Length {
		diffs = append(diffs, fmt.Sprintf("length: %d → %d", a.Length, b.Length))
	}
	if a.Nillable != b.Nillable {
		diffs = append(diffs, fmt.Sprintf("required: %v → %v", !a.Nillable, !b.Nillable))
	}
	if a.Updateable != b.Updateable {
		diffs = append(diffs, fmt.Sprintf("updateable: %v → %v", a.Updateable, b.Updateable))
	}
	if a.Createable != b.Createable {
		diffs = append(diffs, fmt.Sprintf("createable: %v → %v", a.Createable, b.Createable))
	}
	if a.Custom != b.Custom {
		diffs = append(diffs, fmt.Sprintf("custom: %v → %v", a.Custom, b.Custom))
	}
	if a.CalculatedFormula != b.CalculatedFormula {
		diffs = append(diffs, "formula changed")
	}
	if !sameRefs(a.ReferenceTo, b.ReferenceTo) {
		diffs = append(diffs, fmt.Sprintf("references: %v → %v", a.ReferenceTo, b.ReferenceTo))
	}
	if !samePicklist(a.PicklistValues, b.PicklistValues) {
		diffs = append(diffs, "picklist values differ")
	}
	return diffs
}

func sameRefs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

func samePicklist(a, b []PicklistValue) bool {
	mk := func(p []PicklistValue) []string {
		out := make([]string, 0, len(p))
		for _, v := range p {
			if v.Active {
				out = append(out, v.Value)
			}
		}
		sort.Strings(out)
		return out
	}
	x, y := mk(a), mk(b)
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

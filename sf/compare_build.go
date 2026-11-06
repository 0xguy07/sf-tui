package sf

import (
	"fmt"
	"sort"
	"strings"
	"sync"
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
	A          Side              `json:"a"`
	B          Side              `json:"b"`
	Objects    []ObjectState     `json:"objects"`
	FieldLevel bool              `json:"fieldLevel"`       // a field-level compare was requested (--object or --fields)
	Object     string            `json:"object,omitempty"` // the canonical --object name, "" otherwise
	Fields     []ObjectFieldDiff `json:"fields"`
	Summary    Summary           `json:"summary"`
	Different  bool              `json:"different"`
	Errors     []SectionError    `json:"errors"`
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
		// The names-only `sf sobject list` format never sets Custom.
		st.Custom = st.Custom || IsCustomName(n)
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
	if len(a) != len(b) {
		return false
	}
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

// CompareOptions controls BuildSchemaDiff.
type CompareOptions struct {
	Object     string // field-level diff for this one object
	Fields     bool   // field-level diff for every object present on both sides
	CustomOnly bool   // with Fields: only custom objects
	Refresh    bool   // bypass the on-disk caches
	Workers    int    // concurrent describes; <= 0 means 4
}

const defaultCompareWorkers = 4

// BuildSchemaDiff compares two orgs. The error is non-nil only when either
// sobject list cannot be loaded; describe failures are recorded in Errors and
// the remaining objects are still diffed. Fields and Errors are sorted so the
// result is deterministic regardless of worker completion order.
func BuildSchemaDiff(l SchemaLoader, aOrg, bOrg string, opts CompareOptions) (SchemaDiff, error) {
	d := SchemaDiff{
		A: Side{Org: aOrg}, B: Side{Org: bOrg},
		Objects: []ObjectState{}, Fields: []ObjectFieldDiff{}, Errors: []SectionError{},
	}

	// 1. Both sobject lists, concurrently.
	type sideResult struct {
		ss     []SObjectSummary
		cached bool
		err    error
	}
	var ra, rb sideResult
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); ra.ss, ra.cached, ra.err = l.SObjects(aOrg, opts.Refresh) }()
	go func() { defer wg.Done(); rb.ss, rb.cached, rb.err = l.SObjects(bOrg, opts.Refresh) }()
	wg.Wait()
	if ra.err != nil {
		return d, fmt.Errorf("sobjects-a (%s): %w", aOrg, ra.err)
	}
	if rb.err != nil {
		return d, fmt.Errorf("sobjects-b (%s): %w", bOrg, rb.err)
	}
	d.A.SObjects, d.A.Cached = len(ra.ss), ra.cached
	d.B.SObjects, d.B.Cached = len(rb.ss), rb.cached

	// 2. Object level.
	d.Objects, d.Summary = DiffSObjects(ra.ss, rb.ss)
	d.Different = d.Summary.OnlyA > 0 || d.Summary.OnlyB > 0
	stateOf := map[string]ObjectState{}
	for _, o := range d.Objects {
		stateOf[o.Name] = o
	}

	// 3. Field level: pick targets.
	var targets []string
	switch {
	case opts.Object != "":
		d.FieldLevel = true
		name, ok := canonicalObject(d.Objects, opts.Object)
		if !ok {
			return d, fmt.Errorf("%s: not present in either org", opts.Object)
		}
		d.Object = name
		targets = []string{name}
	case opts.Fields:
		d.FieldLevel = true
		for _, o := range d.Objects {
			if o.State != StateBoth {
				continue
			}
			if opts.CustomOnly && !(o.Custom || IsCustomName(o.Name)) {
				continue
			}
			targets = append(targets, o.Name)
		}
	}
	if len(targets) == 0 {
		return d, nil
	}

	workers := opts.Workers
	if workers <= 0 {
		workers = defaultCompareWorkers
	}
	if workers > len(targets) {
		workers = len(targets)
	}
	jobs := make(chan string)
	var mu sync.Mutex
	var pool sync.WaitGroup
	for i := 0; i < workers; i++ {
		pool.Add(1)
		go func() {
			defer pool.Done()
			for name := range jobs {
				fd, errs := describeBoth(l, aOrg, bOrg, name, stateOf[name], opts.Refresh)
				mu.Lock()
				if fd != nil {
					d.Fields = append(d.Fields, *fd)
				}
				d.Errors = append(d.Errors, errs...)
				mu.Unlock()
			}
		}()
	}
	for _, name := range targets {
		jobs <- name
	}
	close(jobs)
	pool.Wait()

	sort.Slice(d.Fields, func(i, j int) bool { return d.Fields[i].SObject < d.Fields[j].SObject })
	sort.Slice(d.Errors, func(i, j int) bool { return d.Errors[i].Section < d.Errors[j].Section })
	d.Summary.ObjectsCompared = len(d.Fields)
	for _, f := range d.Fields {
		if f.Different {
			d.Summary.ObjectsChanged++
			d.Different = true
		}
	}
	return d, nil
}

// describeBoth fetches one object on each side it exists on and diffs it. A
// side where the object is absent, or whose describe fails, yields a
// SectionError. When both sides fail there is nothing to diff.
func describeBoth(l SchemaLoader, aOrg, bOrg, name string, st ObjectState, force bool) (*ObjectFieldDiff, []SectionError) {
	var errs []SectionError
	failed := false
	fetch := func(side, org string, present bool) *Describe {
		if !present {
			// Absent on this side: still diff, listing the other side's fields.
			errs = append(errs, SectionError{Section: "describe-" + side + ":" + name, Message: "not present in " + org})
			return nil
		}
		desc, _, err := l.Describe(org, name, force)
		if err != nil {
			// Failed to load: we can't say what differs, so skip this object.
			errs = append(errs, SectionError{Section: "describe-" + side + ":" + name, Message: err.Error()})
			failed = true
			return nil
		}
		return &desc
	}
	// An unknown object (not in either list) is treated as present on both so
	// the describes run and report their own errors.
	known := st.State != ""
	a := fetch("a", aOrg, !known || st.State != StateOnlyB)
	b := fetch("b", bOrg, !known || st.State != StateOnlyA)
	if failed || (a == nil && b == nil) {
		return nil, errs
	}
	fd := DiffFields(name, a, b)
	return &fd, errs
}

// canonicalObject finds name among the listed objects, case-insensitively,
// and returns the listed spelling.
func canonicalObject(objects []ObjectState, name string) (string, bool) {
	for _, o := range objects {
		if strings.EqualFold(o.Name, name) {
			return o.Name, true
		}
	}
	return "", false
}

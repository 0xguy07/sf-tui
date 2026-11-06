package sf

import (
	"errors"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeSchemaLoader serves fixtures by org and org/sobject, records calls,
// and can inject errors, random delays and a concurrency counter.
type fakeSchemaLoader struct {
	t         *testing.T
	sobjects  map[string]string // org → fixture
	describes map[string]string // org/sobject → fixture
	errs      map[string]error  // org or org/sobject → error
	cachedA   bool
	sleepMax  time.Duration

	mu          sync.Mutex
	calls       []string // "sobjects:org" / "describe:org/sobject"
	forces      []bool
	inFlight    int32
	maxInFlight int32
}

func (f *fakeSchemaLoader) record(call string, force bool) {
	f.mu.Lock()
	f.calls = append(f.calls, call)
	f.forces = append(f.forces, force)
	f.mu.Unlock()
}

func (f *fakeSchemaLoader) SObjects(org string, force bool) ([]SObjectSummary, bool, error) {
	f.record("sobjects:"+org, force)
	if err := f.errs[org]; err != nil {
		return nil, false, err
	}
	file, ok := f.sobjects[org]
	if !ok {
		f.t.Errorf("unexpected sobjects for %s", org)
		return nil, false, errors.New("unexpected")
	}
	return fixtureSObjects(f.t, file), f.cachedA && org == "aorg", nil
}

func (f *fakeSchemaLoader) Describe(org, sobject string, force bool) (Describe, bool, error) {
	f.record("describe:"+org+"/"+sobject, force)
	n := atomic.AddInt32(&f.inFlight, 1)
	for {
		m := atomic.LoadInt32(&f.maxInFlight)
		if n <= m || atomic.CompareAndSwapInt32(&f.maxInFlight, m, n) {
			break
		}
	}
	if f.sleepMax > 0 {
		time.Sleep(time.Duration(rand.Int63n(int64(f.sleepMax))))
	}
	atomic.AddInt32(&f.inFlight, -1)
	if err := f.errs[org+"/"+sobject]; err != nil {
		return Describe{}, false, err
	}
	file, ok := f.describes[org+"/"+sobject]
	if !ok {
		f.t.Errorf("unexpected describe for %s/%s", org, sobject)
		return Describe{}, false, errors.New("unexpected")
	}
	return *fixtureDescribe(f.t, file), false, nil
}

func (f *fakeSchemaLoader) describeCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		if strings.HasPrefix(c, "describe:") {
			out = append(out, c)
		}
	}
	return out
}

func newFake(t *testing.T) *fakeSchemaLoader {
	return &fakeSchemaLoader{
		t:        t,
		sobjects: map[string]string{"aorg": "sobjects_a.json", "borg": "sobjects_b.json"},
		describes: map[string]string{
			"aorg/Account": "describe_account_a.json", "borg/Account": "describe_account_b.json",
			"aorg/Contact": "describe_account_a.json", "borg/Contact": "describe_account_b.json",
			"aorg/Widget__c": "describe_widget_a.json", "borg/Widget__c": "describe_widget_b.json",
			"aorg/Legacy__c": "describe_widget_a.json",
		},
		errs: map[string]error{},
	}
}

func fieldNames(d SchemaDiff) []string {
	var out []string
	for _, f := range d.Fields {
		out = append(out, f.SObject)
	}
	return out
}

func TestBuildSchemaDiff_ObjectsOnly(t *testing.T) {
	f := newFake(t)
	f.cachedA = true
	d, err := BuildSchemaDiff(f, "aorg", "borg", CompareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.describeCalls()) != 0 {
		t.Errorf("no describes expected, got %v", f.describeCalls())
	}
	if d.Summary != (Summary{OnlyA: 1, OnlyB: 1, Both: 3}) || !d.Different {
		t.Errorf("summary=%+v different=%v", d.Summary, d.Different)
	}
	if d.Fields == nil || len(d.Fields) != 0 || d.Errors == nil || !d.A.Cached || d.B.Cached || d.A.SObjects != 4 {
		t.Errorf("sides/fields wrong: %+v", d)
	}
}

func TestBuildSchemaDiff_Object(t *testing.T) {
	d, err := BuildSchemaDiff(newFake(t), "aorg", "borg", CompareOptions{Object: "Widget__c"})
	if err != nil || len(d.Fields) != 1 || d.Summary.ObjectsCompared != 1 || d.Summary.ObjectsChanged != 1 || !d.Fields[0].Different {
		t.Errorf("err=%v diff=%+v", err, d)
	}
}

func TestBuildSchemaDiff_ObjectMissingOnB(t *testing.T) {
	f := newFake(t)
	d, err := BuildSchemaDiff(f, "aorg", "borg", CompareOptions{Object: "Legacy__c"})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Errors) != 1 || d.Errors[0].Section != "describe-b:Legacy__c" || d.Errors[0].Message != "not present in borg" {
		t.Errorf("errors = %+v", d.Errors)
	}
	if len(d.Fields) != 1 || len(d.Fields[0].OnlyA) != 5 || len(d.Fields[0].OnlyB) != 0 {
		t.Errorf("fields = %+v", d.Fields)
	}
	for _, c := range f.describeCalls() {
		if strings.HasPrefix(c, "describe:borg/") {
			t.Errorf("B must not be described for an A-only object: %v", c)
		}
	}
}

func TestBuildSchemaDiff_FieldsDescribesBothSet(t *testing.T) {
	f := newFake(t)
	d, err := BuildSchemaDiff(f, "aorg", "borg", CompareOptions{Fields: true})
	if err != nil {
		t.Fatal(err)
	}
	got := f.describeCalls()
	if len(got) != 6 {
		t.Errorf("want 6 describes (3 objects × 2 orgs), got %v", got)
	}
	for _, bad := range []string{"Legacy__c", "NewThing__c"} {
		for _, c := range got {
			if strings.Contains(c, bad) {
				t.Errorf("one-sided object described: %s", c)
			}
		}
	}
	if d.Summary.ObjectsCompared != 3 || d.Summary.ObjectsChanged != 1 {
		t.Errorf("summary = %+v", d.Summary)
	}
}

func TestBuildSchemaDiff_CustomOnly(t *testing.T) {
	f := newFake(t)
	_, err := BuildSchemaDiff(f, "aorg", "borg", CompareOptions{Fields: true, CustomOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range f.describeCalls() {
		if !strings.HasSuffix(c, "/Widget__c") {
			t.Errorf("non-custom object described: %s", c)
		}
	}
	if len(f.describeCalls()) != 2 {
		t.Errorf("want 2 describes, got %v", f.describeCalls())
	}
}

func TestBuildSchemaDiff_NoCommonObjects(t *testing.T) {
	f := newFake(t)
	f.sobjects["borg"] = "sobjects_disjoint.json"
	d, err := BuildSchemaDiff(f, "aorg", "borg", CompareOptions{Fields: true})
	if err != nil || d.Summary.ObjectsCompared != 0 || len(f.describeCalls()) != 0 {
		t.Errorf("err=%v compared=%d describes=%v", err, d.Summary.ObjectsCompared, f.describeCalls())
	}
}

func TestBuildSchemaDiff_DescribeErrorOneSide(t *testing.T) {
	f := newFake(t)
	f.errs["borg/Contact"] = errors.New("boom")
	d, err := BuildSchemaDiff(f, "aorg", "borg", CompareOptions{Fields: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Errors) != 1 || d.Errors[0].Section != "describe-b:Contact" || !strings.Contains(d.Errors[0].Message, "boom") {
		t.Errorf("errors = %+v", d.Errors)
	}
	if names := fieldNames(d); strings.Join(names, ",") != "Account,Widget__c" {
		t.Errorf("fields = %v", names)
	}
}

func TestBuildSchemaDiff_SortedUnderConcurrency(t *testing.T) {
	for i := 0; i < 5; i++ {
		f := newFake(t)
		f.sleepMax = 20 * time.Millisecond
		d, err := BuildSchemaDiff(f, "aorg", "borg", CompareOptions{Fields: true, Workers: 4})
		if err != nil {
			t.Fatal(err)
		}
		if names := fieldNames(d); strings.Join(names, ",") != "Account,Contact,Widget__c" {
			t.Fatalf("run %d: fields = %v", i, names)
		}
	}
}

func TestBuildSchemaDiff_WorkersBound(t *testing.T) {
	f := newFake(t)
	f.sleepMax = 10 * time.Millisecond
	if _, err := BuildSchemaDiff(f, "aorg", "borg", CompareOptions{Fields: true, Workers: 2}); err != nil {
		t.Fatal(err)
	}
	if f.maxInFlight > 2 {
		t.Errorf("max concurrent describes = %d, want ≤ 2", f.maxInFlight)
	}
}

func TestBuildSchemaDiff_RefreshForces(t *testing.T) {
	f := newFake(t)
	if _, err := BuildSchemaDiff(f, "aorg", "borg", CompareOptions{Fields: true, Refresh: true}); err != nil {
		t.Fatal(err)
	}
	for i, fo := range f.forces {
		if !fo {
			t.Errorf("call %d (%s) not forced", i, f.calls[i])
		}
	}
}

func TestBuildSchemaDiff_SObjectsFail(t *testing.T) {
	f := newFake(t)
	f.errs["aorg"] = errors.New("boom")
	if _, err := BuildSchemaDiff(f, "aorg", "borg", CompareOptions{}); err == nil || !strings.Contains(err.Error(), "sobjects-a") {
		t.Errorf("err = %v", err)
	}
}

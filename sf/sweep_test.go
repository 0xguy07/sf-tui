package sf

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestSFCommandError_SinglePrefix(t *testing.T) {
	err := sfCommandError("sf sobject list", nil, errors.New(`exec: "sf": executable file not found in $PATH`))
	if !strings.HasPrefix(err.Error(), "sf sobject list: exec:") {
		t.Errorf("want one command prefix, got %q", err.Error())
	}
	js := []byte(`{"status":1,"name":"INVALID_FIELD","message":"bad column"}`)
	if got := sfCommandError("sf data query", js, errors.New("exit status 1")).Error(); got != "sf data query: INVALID_FIELD: bad column" {
		t.Errorf("got %q", got)
	}
}

type allowGate struct{}

func (allowGate) Check(string, WriteAction) error          { return nil }
func (allowGate) Confirmation(string, WriteAction) Confirm { return ConfirmNone }

func TestRunQuery_SfMissing_UsesCommandError(t *testing.T) {
	isolate(t)
	msg := RunQuery("x", "SELECT Id FROM Account")()
	em, ok := msg.(ErrMsg)
	if !ok || !strings.HasPrefix(em.Error(), "sf data query: ") || !strings.Contains(em.Error(), "executable file not found") {
		t.Errorf("got %#v", msg)
	}
}

func TestUpdateRecord_SfMissing_UsesCommandError(t *testing.T) {
	isolate(t)
	msg := UpdateRecord(allowGate{}, "x", "Account", "001000000000001AAA", map[string]string{"Name": "x"})()
	em, ok := msg.(ErrMsg)
	if !ok || !strings.HasPrefix(em.Error(), "sf data update record: ") || !strings.Contains(em.Error(), "executable file not found") {
		t.Errorf("got %#v", msg)
	}
}

func TestDiffSObjects_CustomFromName(t *testing.T) {
	// The names-only `sf sobject list` format never sets Custom.
	states, _ := DiffSObjects([]SObjectSummary{{Name: "Widget__c"}, {Name: "Account"}}, nil)
	if !states[1].Custom || states[0].Custom {
		t.Errorf("custom flags = %+v", states)
	}
}

func TestSoqlEscape_Backslash(t *testing.T) {
	if got := soqlEscape(`a\'b`); got != `a\\\'b` {
		t.Errorf("got %q", got)
	}
}

func TestTrace_TrackedFieldsNeverNull(t *testing.T) {
	var tr Trace
	tr.ApplyHistory(HistoryPart{})
	b, _ := json.Marshal(tr)
	if !strings.Contains(string(b), `"trackedFields":[]`) {
		t.Errorf("trackedFields should be [], got %s", b)
	}
}

func TestBuildSchemaDiff_SObjectsFail_NamesOrg(t *testing.T) {
	f := newFake(t)
	f.errs["aorg"] = errors.New("boom")
	if _, err := BuildSchemaDiff(f, "aorg", "borg", CompareOptions{}); err == nil || !strings.Contains(err.Error(), "sobjects-a (aorg)") {
		t.Errorf("err = %v", err)
	}
}

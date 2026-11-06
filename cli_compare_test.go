package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/0xguy07/sf-tui/sf"
)

type compareCall struct {
	a, b string
	opts sf.CompareOptions
}

func compareDeps(d sf.SchemaDiff, err error) (cliDeps, *[]compareCall) {
	calls := &[]compareCall{}
	return cliDeps{compare: func(a, b string, opts sf.CompareOptions) (sf.SchemaDiff, error) {
		*calls = append(*calls, compareCall{a, b, opts})
		return d, err
	}}, calls
}

func runCompareCapture(t *testing.T, d cliDeps, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := runCompare(args, &out, &errb, d)
	return code, out.String(), errb.String()
}

func TestCompare_TwoOrgsRequired(t *testing.T) {
	d, calls := compareDeps(fixtureDiff(t), nil)
	code, so, se := runCompareCapture(t, d, "onlyone")
	if code != 2 || so != "" || !strings.Contains(se, "need two orgs") || len(*calls) != 0 {
		t.Errorf("exit=%d stdout=%q stderr=%q", code, so, se)
	}
}

func TestCompare_SameOrgRejected(t *testing.T) {
	d, calls := compareDeps(fixtureDiff(t), nil)
	if code, _, se := runCompareCapture(t, d, "prod", "prod"); code != 2 || !strings.Contains(se, "same org") || len(*calls) != 0 {
		t.Errorf("exit=%d stderr=%q", code, se)
	}
}

func TestCompare_OrgsBeforeOrAfterFlags(t *testing.T) {
	for _, args := range [][]string{{"a", "b", "--fields"}, {"--fields", "a", "b"}} {
		d, calls := compareDeps(fixtureDiff(t), nil)
		if code, _, se := runCompareCapture(t, d, args...); code != 0 {
			t.Fatalf("%v: exit %d, stderr %q", args, code, se)
		}
		if len(*calls) != 1 || (*calls)[0].a != "a" || (*calls)[0].b != "b" || !(*calls)[0].opts.Fields {
			t.Errorf("%v: calls = %+v", args, *calls)
		}
	}
}

func TestCompare_ObjectAndFieldsExclusive(t *testing.T) {
	d, _ := compareDeps(fixtureDiff(t), nil)
	if code, _, se := runCompareCapture(t, d, "a", "b", "--object", "Account", "--fields"); code != 2 || !strings.Contains(se, "--object") {
		t.Errorf("exit=%d stderr=%q", code, se)
	}
}

func TestCompare_CustomOnlyNeedsFields(t *testing.T) {
	d, _ := compareDeps(fixtureDiff(t), nil)
	if code, _, se := runCompareCapture(t, d, "a", "b", "--custom-only"); code != 2 || !strings.Contains(se, "--custom-only") {
		t.Errorf("exit=%d stderr=%q", code, se)
	}
}

func TestCompare_ExitCodeOnDrift(t *testing.T) {
	d, _ := compareDeps(fixtureDiff(t), nil)
	if code, so, _ := runCompareCapture(t, d, "a", "b", "--exit-code"); code != 3 || so == "" {
		t.Errorf("with --exit-code: exit=%d stdout empty=%v", code, so == "")
	}
	if code, _, _ := runCompareCapture(t, d, "a", "b"); code != 0 {
		t.Errorf("without --exit-code: exit=%d, want 0", code)
	}
}

func TestCompare_ExitCodeErrorsOnlyIsZero(t *testing.T) {
	diff := fixtureDiff(t)
	diff.Different = false
	diff.Errors = []sf.SectionError{{Section: "describe-b:Contact", Message: "boom"}}
	d, _ := compareDeps(diff, nil)
	if code, _, se := runCompareCapture(t, d, "a", "b", "--exit-code"); code != 0 || !strings.Contains(se, "warning: describe-b:Contact: boom") {
		t.Errorf("exit=%d stderr=%q", code, se)
	}
}

func TestCompare_RefreshAndWorkersDefault(t *testing.T) {
	d, calls := compareDeps(fixtureDiff(t), nil)
	runCompareCapture(t, d, "a", "b", "--refresh")
	want := sf.CompareOptions{Refresh: true, Workers: 4}
	if !reflect.DeepEqual((*calls)[0].opts, want) {
		t.Errorf("opts = %+v, want %+v", (*calls)[0].opts, want)
	}
}

func TestCompare_RuntimeErrorExit1(t *testing.T) {
	d, _ := compareDeps(sf.SchemaDiff{}, errors.New(`sobjects-a: sf sobject list: exec: "sf": executable file not found`))
	if code, so, se := runCompareCapture(t, d, "a", "b"); code != 1 || so != "" || !strings.Contains(se, "executable file not found") {
		t.Errorf("exit=%d stdout=%q stderr=%q", code, so, se)
	}
}

func TestCompare_JSONSingleDocWithWarnings(t *testing.T) {
	diff := fixtureDiff(t)
	diff.Errors = []sf.SectionError{{Section: "describe-a:Widget__c", Message: "boom"}}
	d, _ := compareDeps(diff, nil)
	code, so, se := runCompareCapture(t, d, "a", "b", "--format", "json")
	var got sf.SchemaDiff
	if code != 0 || json.Unmarshal([]byte(so), &got) != nil || !hasLine(se, "warning: describe-a:Widget__c: boom") {
		t.Errorf("exit=%d stderr=%q", code, se)
	}
}

func TestCompare_TextMatchesRenderer(t *testing.T) {
	diff := fixtureDiff(t)
	d, _ := compareDeps(diff, nil)
	if _, so, _ := runCompareCapture(t, d, "a", "b"); so != renderSchemaDiffText(diff) {
		t.Error("text stdout differs from renderer")
	}
}

func TestCompare_JSONRoundTrips(t *testing.T) {
	diff := fixtureDiff(t)
	d, _ := compareDeps(diff, nil)
	_, so, _ := runCompareCapture(t, d, "a", "b", "--format", "json")
	var got sf.SchemaDiff
	if err := json.Unmarshal([]byte(so), &got); err != nil {
		t.Fatal(err)
	}
	x, _ := json.Marshal(got)
	y, _ := json.Marshal(diff)
	if !bytes.Equal(x, y) {
		t.Error("JSON does not round-trip")
	}
}

func TestSplitPositionalN(t *testing.T) {
	pos, rest := splitPositionalN([]string{"a", "b", "--x"}, 2)
	if !reflect.DeepEqual(pos, []string{"a", "b"}) || !reflect.DeepEqual(rest, []string{"--x"}) {
		t.Errorf("got %v %v", pos, rest)
	}
	pos, rest = splitPositionalN([]string{"a", "b", "c"}, 2)
	if !reflect.DeepEqual(pos, []string{"a", "b"}) || !reflect.DeepEqual(rest, []string{"--", "c"}) {
		t.Errorf("got %v %v", pos, rest)
	}
	pos, rest = splitPositionalN([]string{"--fields", "a"}, 2)
	if !reflect.DeepEqual(pos, []string{"a"}) || !reflect.DeepEqual(rest, []string{"--fields"}) {
		t.Errorf("got %v %v", pos, rest)
	}
}

func TestRunCLI_CompareDispatch(t *testing.T) {
	var out, errb bytes.Buffer
	handled, code := runCLI([]string{"compare"}, &out, &errb, cliDeps{})
	if !handled || code != 2 || !strings.Contains(errb.String(), "sftui compare") {
		t.Errorf("handled=%v code=%d stderr=%q", handled, code, errb.String())
	}
}

func TestCompare_ObjectNothingComparedExit1(t *testing.T) {
	diff := fixtureDiff(t)
	diff.FieldLevel, diff.Object, diff.Fields = true, "Widget__c", []sf.ObjectFieldDiff{}
	diff.Errors = []sf.SectionError{{Section: "describe-a:Widget__c", Message: "boom"}, {Section: "describe-b:Widget__c", Message: "boom"}}
	d, _ := compareDeps(diff, nil)
	code, so, se := runCompareCapture(t, d, "a", "b", "--object", "Widget__c", "--exit-code")
	if code != 1 || so != "" || !strings.Contains(se, "could not compare Widget__c") {
		t.Errorf("exit=%d stdout=%q stderr=%q", code, so, se)
	}
}

func TestSplitPositionalN_DoubleDash(t *testing.T) {
	pos, rest := splitPositionalN([]string{"--fields", "--", "a", "b"}, 2)
	if !reflect.DeepEqual(pos, []string{"a", "b"}) || !reflect.DeepEqual(rest, []string{"--fields"}) {
		t.Errorf("got %v %v", pos, rest)
	}
}

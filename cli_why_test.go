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

const goodID = "001000000000001AAA"

type traceCall struct {
	org, id string
	refresh bool
}

// stubDeps returns deps whose trace func returns (tr, err) and records its
// calls; defaultOrg returns dflt.
func stubDeps(tr sf.Trace, err error, dflt string) (cliDeps, *[]traceCall) {
	calls := &[]traceCall{}
	return cliDeps{
		trace: func(org, id string, refresh bool) (sf.Trace, error) {
			*calls = append(*calls, traceCall{org, id, refresh})
			return tr, err
		},
		defaultOrg: func() (string, error) { return dflt, nil },
	}, calls
}

func runWhyCapture(t *testing.T, d cliDeps, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = runWhy(args, &out, &errb, d)
	return code, out.String(), errb.String()
}

func TestWhy_IdBeforeOrAfterFlags(t *testing.T) {
	for _, args := range [][]string{{goodID, "--org", "x"}, {"--org", "x", goodID}} {
		d, calls := stubDeps(fixtureTrace(t), nil, "")
		if code, _, se := runWhyCapture(t, d, args...); code != 0 {
			t.Fatalf("%v: exit %d, stderr %q", args, code, se)
		}
		if want := []traceCall{{"x", goodID, false}}; !reflect.DeepEqual(*calls, want) {
			t.Errorf("%v: calls = %+v, want %+v", args, *calls, want)
		}
	}
}

func TestWhy_TrimsId(t *testing.T) {
	d, calls := stubDeps(fixtureTrace(t), nil, "")
	if code, _, se := runWhyCapture(t, d, " "+goodID+" ", "--org", "x"); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, se)
	}
	if (*calls)[0].id != goodID {
		t.Errorf("id = %q, want trimmed", (*calls)[0].id)
	}
}

func TestWhy_InvalidId(t *testing.T) {
	d, calls := stubDeps(fixtureTrace(t), nil, "")
	code, so, se := runWhyCapture(t, d, "nope", "--org", "x")
	if code != 2 || !strings.Contains(se, "invalid record Id") || so != "" || len(*calls) != 0 {
		t.Errorf("exit=%d stdout=%q stderr=%q calls=%d", code, so, se, len(*calls))
	}
}

func TestWhy_FormatCaseInsensitive(t *testing.T) {
	d, _ := stubDeps(fixtureTrace(t), nil, "")
	code, so, _ := runWhyCapture(t, d, goodID, "--org", "x", "--format", "JSON")
	if code != 0 || !json.Valid([]byte(so)) {
		t.Errorf("exit=%d, stdout valid JSON=%v", code, json.Valid([]byte(so)))
	}
}

func TestWhy_BadFormat(t *testing.T) {
	d, _ := stubDeps(fixtureTrace(t), nil, "")
	if code, _, _ := runWhyCapture(t, d, goodID, "--org", "x", "--format", "yaml"); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}

func TestWhy_DefaultOrgFallback(t *testing.T) {
	d, calls := stubDeps(fixtureTrace(t), nil, "dflt")
	if code, _, se := runWhyCapture(t, d, goodID); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, se)
	}
	if (*calls)[0].org != "dflt" {
		t.Errorf("org = %q, want dflt", (*calls)[0].org)
	}
}

func TestWhy_NoOrg(t *testing.T) {
	d, calls := stubDeps(fixtureTrace(t), nil, "")
	code, so, se := runWhyCapture(t, d, goodID)
	if code != 2 || !strings.Contains(se, "sf config set target-org") || so != "" || len(*calls) != 0 {
		t.Errorf("exit=%d stdout=%q stderr=%q calls=%d", code, so, se, len(*calls))
	}
}

func TestWhy_RefreshFlag(t *testing.T) {
	d, calls := stubDeps(fixtureTrace(t), nil, "")
	runWhyCapture(t, d, goodID, "--org", "x", "--refresh")
	if !(*calls)[0].refresh {
		t.Error("refresh not passed through")
	}
}

func TestWhy_TraceErrorExit1(t *testing.T) {
	d, _ := stubDeps(sf.Trace{}, errors.New("sf org display: no access token for x"), "")
	code, so, se := runWhyCapture(t, d, goodID, "--org", "x")
	if code != 1 || !strings.Contains(se, "no access token") || so != "" {
		t.Errorf("exit=%d stdout=%q stderr=%q", code, so, se)
	}
}

func TestWhy_PartialResultsExit0(t *testing.T) {
	tr := fixtureTrace(t)
	tr.Errors = []sf.SectionError{{Section: "automation", Message: "boom"}}
	d, _ := stubDeps(tr, nil, "")
	code, so, se := runWhyCapture(t, d, goodID, "--org", "x", "--format", "json")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	var got sf.Trace
	if err := json.Unmarshal([]byte(so), &got); err != nil {
		t.Fatalf("stdout is not one JSON document: %v", err)
	}
	if len(got.Errors) != 1 || got.Errors[0].Message != "boom" {
		t.Errorf("errors = %+v", got.Errors)
	}
	if !strings.Contains(se, "warning: automation: boom") {
		t.Errorf("stderr = %q", se)
	}
}

func TestWhy_TextMatchesRenderer(t *testing.T) {
	tr := fixtureTrace(t)
	d, _ := stubDeps(tr, nil, "")
	_, so, _ := runWhyCapture(t, d, goodID, "--org", "x")
	if so != renderTraceText(tr) {
		t.Error("text stdout differs from renderTraceText")
	}
}

func TestWhy_JSONRoundTrips(t *testing.T) {
	tr := fixtureTrace(t)
	d, _ := stubDeps(tr, nil, "")
	_, so, _ := runWhyCapture(t, d, goodID, "--org", "x", "--format", "json")
	var got sf.Trace
	if err := json.Unmarshal([]byte(so), &got); err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(got)
	b, _ := json.Marshal(tr)
	if !bytes.Equal(a, b) {
		t.Error("JSON output does not round-trip to the input Trace")
	}
}

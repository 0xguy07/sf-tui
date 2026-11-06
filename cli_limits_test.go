package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/0xguy07/sf-tui/sf"
)

func limitsDeps(t *testing.T, ls []sf.Limit, err error) (cliDeps, *[]string) {
	orgs := &[]string{}
	return cliDeps{
		defaultOrg: func() (string, error) { return "dflt", nil },
		limits: func(org string) ([]sf.Limit, error) {
			*orgs = append(*orgs, org)
			return ls, err
		},
	}, orgs
}

func rawLimits(t *testing.T) []sf.Limit {
	t.Helper()
	r := fixtureLimits(t, nil)
	out := make([]sf.Limit, 0, len(r.Limits))
	for _, l := range r.Limits {
		out = append(out, sf.Limit{Name: l.Name, Max: l.Max, Remaining: l.Remaining})
	}
	return out
}

func runLimitsCapture(t *testing.T, d cliDeps, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := runLimits(args, &out, &errb, d)
	return code, out.String(), errb.String()
}

func TestLimits_WarnValidation(t *testing.T) {
	d, orgs := limitsDeps(t, rawLimits(t), nil)
	for _, v := range []string{"0", "101", "abc", "-5", "50.5"} {
		if code, _, se := runLimitsCapture(t, d, "--org", "x", "--warn", v); code != 2 || !strings.Contains(se, "--warn") {
			t.Errorf("--warn %q: exit=%d stderr=%q", v, code, se)
		}
	}
	if len(*orgs) != 0 {
		t.Error("invalid --warn must not call sf")
	}
}

func TestLimits_ExitCodeOnBreach(t *testing.T) {
	d, _ := limitsDeps(t, rawLimits(t), nil)
	if code, so, _ := runLimitsCapture(t, d, "--org", "x", "--warn", "90"); code != 3 || !strings.Contains(so, "2 of 5 limits at or above 90%") {
		t.Errorf("exit=%d stdout=%q", code, so)
	}
	if code, _, _ := runLimitsCapture(t, d, "--org", "x", "--warn", "100"); code != 3 {
		t.Errorf("exactly 100%% should breach --warn 100, exit=%d", code)
	}
	if code, so, _ := runLimitsCapture(t, d, "--org", "x", "--warn", "101"); code != 2 || so != "" {
		t.Errorf("101 invalid: exit=%d", code)
	}
}

func TestLimits_NoBreachOrNoWarnExit0(t *testing.T) {
	d, _ := limitsDeps(t, rawLimits(t), nil)
	if code, _, _ := runLimitsCapture(t, d, "--org", "x"); code != 0 {
		t.Errorf("no --warn: exit=%d", code)
	}
	ls := []sf.Limit{{Name: "A", Max: 100, Remaining: 90}}
	d, _ = limitsDeps(t, ls, nil)
	if code, so, _ := runLimitsCapture(t, d, "--org", "x", "--warn", "50"); code != 0 || !strings.Contains(so, "0 of 1 limits at or above 50%") {
		t.Errorf("under threshold: exit=%d stdout=%q", code, so)
	}
}

func TestLimits_EmptyLimits(t *testing.T) {
	d, _ := limitsDeps(t, []sf.Limit{}, nil)
	code, so, _ := runLimitsCapture(t, d, "--org", "x", "--warn", "80")
	if code != 0 || !strings.Contains(so, "(0 limits)") || !strings.Contains(so, "0 of 0 limits at or above 80%") {
		t.Errorf("exit=%d stdout=%q", code, so)
	}
}

func TestLimits_DefaultOrgFallback(t *testing.T) {
	d, orgs := limitsDeps(t, rawLimits(t), nil)
	if code, _, se := runLimitsCapture(t, d); code != 0 || (*orgs)[0] != "dflt" {
		t.Errorf("exit=%d orgs=%v stderr=%q", code, *orgs, se)
	}
}

func TestLimits_RuntimeErrorExit1(t *testing.T) {
	d, _ := limitsDeps(t, nil, errors.New("sf limits api display: timed out after 1s"))
	if code, so, se := runLimitsCapture(t, d, "--org", "x"); code != 1 || so != "" || !strings.Contains(se, "timed out") {
		t.Errorf("exit=%d stdout=%q stderr=%q", code, so, se)
	}
}

func TestLimits_JSONSingleDocAndRoundTrip(t *testing.T) {
	d, _ := limitsDeps(t, rawLimits(t), nil)
	code, so, _ := runLimitsCapture(t, d, "--org", "x", "--warn", "90", "--format", "json")
	if code != 3 {
		t.Fatalf("exit=%d", code)
	}
	var got sf.LimitsReport
	if err := json.Unmarshal([]byte(so), &got); err != nil {
		t.Fatal(err)
	}
	if got.Warn == nil || *got.Warn != 90 || len(got.Breached) != 2 || got.Org != "x" || len(got.Limits) != 5 {
		t.Errorf("got %+v", got)
	}
	if !strings.Contains(so, `"breached": [`) {
		t.Error("breached must be an array")
	}
}

func TestLimits_TextMatchesRenderer(t *testing.T) {
	d, _ := limitsDeps(t, rawLimits(t), nil)
	_, so, _ := runLimitsCapture(t, d, "--org", "prod", "--warn", "90")
	if so != renderLimitsText(fixtureLimits(t, intp(90))) {
		t.Error("text stdout differs from renderer")
	}
}

func TestRunCLI_LimitsDispatch(t *testing.T) {
	var out, errb bytes.Buffer
	d, _ := limitsDeps(t, rawLimits(t), nil)
	d.defaultOrg = func() (string, error) { return "", nil }
	handled, code := runCLI([]string{"limits"}, &out, &errb, d)
	if !handled || code != 2 || !strings.Contains(errb.String(), "sf config set target-org") {
		t.Errorf("handled=%v code=%d stderr=%q", handled, code, errb.String())
	}
	if !strings.Contains(usage(), "sftui limits") {
		t.Error("usage should mention sftui limits")
	}
}

func TestLimits_PositionalOrgHint(t *testing.T) {
	d, orgs := limitsDeps(t, rawLimits(t), nil)
	if code, _, se := runLimitsCapture(t, d, "prod"); code != 2 || !strings.Contains(se, "--org prod") || len(*orgs) != 0 {
		t.Errorf("exit=%d stderr=%q", code, se)
	}
}

func TestLimits_JSONHasErrorsArray(t *testing.T) {
	d, _ := limitsDeps(t, rawLimits(t), nil)
	_, so, _ := runLimitsCapture(t, d, "--org", "x", "--format", "json")
	if !strings.Contains(so, `"errors": []`) {
		t.Errorf("JSON should carry errors: [] like the other commands:\n%s", so)
	}
}

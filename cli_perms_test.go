package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/0xguy07/sf-tui/sf"
)

type permsCall struct {
	org, object string
	user        sf.UserBrief
}

func permsDeps(t *testing.T, r sf.PermissionsReport, permsErr error, resolveErr error) (cliDeps, *[]permsCall) {
	calls := &[]permsCall{}
	return cliDeps{
		defaultOrg: func() (string, error) { return "", nil },
		resolveUser: func(org, ident string) (sf.UserBrief, error) {
			if resolveErr != nil {
				return sf.UserBrief{}, resolveErr
			}
			return r.User, nil
		},
		perms: func(org string, user sf.UserBrief, object string) (sf.PermissionsReport, error) {
			*calls = append(*calls, permsCall{org, object, user})
			return r, permsErr
		},
	}, calls
}

func runPermsCapture(t *testing.T, d cliDeps, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := runPerms(args, &out, &errb, d)
	return code, out.String(), errb.String()
}

func TestPerms_IdentBeforeOrAfterFlags(t *testing.T) {
	for _, args := range [][]string{{"alex@acme.example", "--org", "x"}, {"--org", "x", "alex@acme.example"}} {
		d, calls := permsDeps(t, fixtureReport(t), nil, nil)
		if code, _, se := runPermsCapture(t, d, args...); code != 0 {
			t.Fatalf("%v: exit %d, stderr %q", args, code, se)
		}
		if len(*calls) != 1 || (*calls)[0].org != "x" || (*calls)[0].user.ID != "005000000000001AAA" {
			t.Errorf("%v: calls = %+v", args, *calls)
		}
	}
}

func TestPerms_TrimsIdent(t *testing.T) {
	var got string
	d, _ := permsDeps(t, fixtureReport(t), nil, nil)
	d.resolveUser = func(org, ident string) (sf.UserBrief, error) { got = ident; return fixtureReport(t).User, nil }
	runPermsCapture(t, d, " alex@acme.example ", "--org", "x")
	if got != "alex@acme.example" {
		t.Errorf("ident = %q, want trimmed", got)
	}
}

func TestPerms_MissingIdent(t *testing.T) {
	d, calls := permsDeps(t, fixtureReport(t), nil, nil)
	code, so, se := runPermsCapture(t, d, "--org", "x")
	if code != 2 || so != "" || !strings.Contains(se, "missing user") || len(*calls) != 0 {
		t.Errorf("exit=%d stdout=%q stderr=%q", code, so, se)
	}
}

func TestPerms_ObjectFlagPassedThrough(t *testing.T) {
	d, calls := permsDeps(t, fixtureReport(t), nil, nil)
	runPermsCapture(t, d, "alex@acme.example", "--org", "x", "--object", "Account")
	if len(*calls) != 1 || (*calls)[0].object != "Account" {
		t.Errorf("calls = %+v", *calls)
	}
}

func TestPerms_AmbiguousUserExit2(t *testing.T) {
	amb := &sf.ErrAmbiguousUser{Ident: "Alex Admin", Candidates: []sf.UserBrief{
		{Name: "Alex Admin", Username: "alex@acme.example", ID: "005000000000001AAA"},
		{Name: "Alex Admin", Username: "alex.admin@acme.example", ID: "005000000000002AAA"},
	}}
	d, calls := permsDeps(t, fixtureReport(t), nil, amb)
	code, so, se := runPermsCapture(t, d, "Alex Admin", "--org", "x")
	if code != 2 || so != "" || len(*calls) != 0 {
		t.Errorf("exit=%d stdout=%q calls=%d", code, so, len(*calls))
	}
	for _, want := range []string{"alex@acme.example", "alex.admin@acme.example", "pass the username or Id"} {
		if !strings.Contains(se, want) {
			t.Errorf("stderr missing %q: %q", want, se)
		}
	}
}

func TestPerms_UnknownUserExit2(t *testing.T) {
	d, _ := permsDeps(t, fixtureReport(t), nil, errors.New("no user matches \"nobody\""))
	d.resolveUser = func(string, string) (sf.UserBrief, error) { return sf.UserBrief{}, sf.ErrUserNotFound }
	if code, _, se := runPermsCapture(t, d, "nobody", "--org", "x"); code != 2 || !strings.Contains(se, "no user matches") {
		t.Errorf("exit=%d stderr=%q", code, se)
	}
}

func TestPerms_ResolveRuntimeErrorExit1(t *testing.T) {
	d, _ := permsDeps(t, fixtureReport(t), nil, errors.New(`sf data query: exec: "sf": executable file not found`))
	code, so, se := runPermsCapture(t, d, "alex@acme.example", "--org", "x")
	if code != 1 || so != "" || !strings.Contains(se, "executable file not found") {
		t.Errorf("exit=%d stdout=%q stderr=%q", code, so, se)
	}
}

func TestPerms_PartialResultsExit0(t *testing.T) {
	r := fixtureReport(t)
	r.Errors = []sf.SectionError{{Section: "objects", Message: "boom"}}
	d, _ := permsDeps(t, r, nil, nil)
	code, so, se := runPermsCapture(t, d, "alex@acme.example", "--org", "x", "--format", "json")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var got sf.PermissionsReport
	if err := json.Unmarshal([]byte(so), &got); err != nil {
		t.Fatalf("stdout not one JSON doc: %v", err)
	}
	if !strings.Contains(se, "warning: objects: boom") {
		t.Errorf("stderr = %q", se)
	}
}

func TestPerms_TextMatchesRenderer(t *testing.T) {
	r := fixtureReport(t)
	d, _ := permsDeps(t, r, nil, nil)
	_, so, _ := runPermsCapture(t, d, "alex@acme.example", "--org", "x")
	if so != renderPermissionsText(r) {
		t.Error("text stdout differs from renderPermissionsText")
	}
}

func TestPerms_JSONRoundTrips(t *testing.T) {
	r := fixtureReport(t)
	d, _ := permsDeps(t, r, nil, nil)
	_, so, _ := runPermsCapture(t, d, "alex@acme.example", "--org", "x", "--format", "json")
	var got sf.PermissionsReport
	if err := json.Unmarshal([]byte(so), &got); err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(got)
	b, _ := json.Marshal(r)
	if !bytes.Equal(a, b) {
		t.Error("JSON does not round-trip")
	}
}

func TestRunCLI_PermsDispatch(t *testing.T) {
	var out, errb bytes.Buffer
	handled, code := runCLI([]string{"perms"}, &out, &errb, cliDeps{})
	if !handled || code != 2 || !strings.Contains(errb.String(), "sftui perms") {
		t.Errorf("handled=%v code=%d stderr=%q", handled, code, errb.String())
	}
}

package sf

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

type soqlRoute struct {
	match string
	file  string
	err   error
}

type fakeSOQL struct {
	t      *testing.T
	routes []soqlRoute
	seen   []string
}

func (f *fakeSOQL) SOQL(org, soql string, tooling bool) ([]byte, error) {
	f.seen = append(f.seen, soql)
	for _, r := range f.routes {
		if strings.Contains(soql, r.match) {
			if r.err != nil {
				return nil, r.err
			}
			return permsFixture(f.t, r.file), nil
		}
	}
	f.t.Errorf("unexpected query: %s", soql)
	return nil, errors.New("unexpected query")
}

func alexUser() UserBrief {
	return UserBrief{ID: "005000000000001AAA", Name: "Alex Admin", Username: "alex@acme.example", IsActive: true, ProfileName: "System Administrator"}
}

func TestResolveUser_ById(t *testing.T) {
	q := &fakeSOQL{t: t, routes: []soqlRoute{{match: "WHERE Id = '005000000000001AAA'", file: "user_by_id.json"}}}
	u, err := ResolveUser(q, "o", "005000000000001AAA")
	if err != nil || u != alexUser() {
		t.Errorf("got %+v, %v", u, err)
	}
}

func TestResolveUser_ByUsername(t *testing.T) {
	q := &fakeSOQL{t: t, routes: []soqlRoute{{match: "WHERE Username = 'alex@acme.example'", file: "user_by_username.json"}}}
	u, err := ResolveUser(q, "o", "ALEX@acme.example")
	if err != nil || u.ID != "005000000000001AAA" {
		t.Errorf("got %+v, %v", u, err)
	}
}

func TestResolveUser_ByNameAmbiguous(t *testing.T) {
	q := &fakeSOQL{t: t, routes: []soqlRoute{{match: "WHERE Name = 'Alex Admin'", file: "users_name_ambiguous.json"}}}
	_, err := ResolveUser(q, "o", "Alex Admin")
	var amb *ErrAmbiguousUser
	if !errors.As(err, &amb) || len(amb.Candidates) != 2 {
		t.Fatalf("want ErrAmbiguousUser with 2 candidates, got %v", err)
	}
	if msg := err.Error(); !strings.Contains(msg, "alex@acme.example") || !strings.Contains(msg, "alex.admin@acme.example") || !strings.Contains(msg, "005000000000002AAA") {
		t.Errorf("message should list candidates: %q", msg)
	}
}

func TestResolveUser_None(t *testing.T) {
	q := &fakeSOQL{t: t, routes: []soqlRoute{{match: "FROM User", file: "users_none.json"}}}
	if _, err := ResolveUser(q, "o", "Nobody Here"); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("want ErrUserNotFound, got %v", err)
	}
}

func TestResolveUser_EscapesQuote(t *testing.T) {
	q := &fakeSOQL{t: t, routes: []soqlRoute{{match: "FROM User", file: "users_none.json"}}}
	ResolveUser(q, "o", "O'Brien")
	if len(q.seen) != 1 || !strings.Contains(q.seen[0], `O\'Brien`) {
		t.Errorf("query not escaped: %v", q.seen)
	}
}

func fullRoutes() []soqlRoute {
	return []soqlRoute{
		{match: "FROM PermissionSetAssignment", file: "assignments.json"},
		{match: "FROM ObjectPermissions", file: "objectperms.json"},
		{match: "FROM FieldPermissions", file: "fieldperms.json"},
	}
}

func TestBuildPermissionsReport_Full(t *testing.T) {
	q := &fakeSOQL{t: t, routes: fullRoutes()}
	r, err := BuildPermissionsReport(q, "o", alexUser(), "Account")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Assignments) != 4 || len(r.Objects) != 3 || len(r.Fields) != 2 || r.Object != "Account" {
		t.Errorf("counts: %d assignments, %d objects, %d fields, object %q", len(r.Assignments), len(r.Objects), len(r.Fields), r.Object)
	}
	if len(r.Overrides.ModifyAllData) != 1 || r.Errors == nil || len(r.Errors) != 0 {
		t.Errorf("overrides=%+v errors=%#v", r.Overrides, r.Errors)
	}
	if r.User != alexUser() {
		t.Errorf("user = %+v", r.User)
	}
}

func TestBuildPermissionsReport_NoObject(t *testing.T) {
	q := &fakeSOQL{t: t, routes: fullRoutes()}
	r, err := BuildPermissionsReport(q, "o", alexUser(), "")
	if err != nil || r.Fields == nil || len(r.Fields) != 0 || r.Object != "" {
		t.Errorf("fields=%#v object=%q err=%v (want empty non-nil fields)", r.Fields, r.Object, err)
	}
	for _, s := range q.seen {
		if strings.Contains(s, "FROM FieldPermissions") {
			t.Error("FieldPermissions should not be queried without --object")
		}
	}
}

func TestBuildPermissionsReport_ObjectPermsFail(t *testing.T) {
	routes := fullRoutes()
	routes[1].err = errors.New("boom")
	q := &fakeSOQL{t: t, routes: routes}
	r, err := BuildPermissionsReport(q, "o", alexUser(), "")
	if err != nil {
		t.Fatalf("partial failure must not be a hard error: %v", err)
	}
	if len(r.Errors) != 1 || r.Errors[0].Section != "objects" || !strings.Contains(r.Errors[0].Message, "boom") {
		t.Errorf("errors = %+v", r.Errors)
	}
	if len(r.Assignments) != 4 || r.Objects == nil {
		t.Errorf("rest of report should stand: %+v", r)
	}
}

func TestBuildPermissionsReport_GroupRetry(t *testing.T) {
	q := &fakeSOQL{t: t, routes: []soqlRoute{
		{match: "PermissionSetGroup.MasterLabel", err: errors.New("INVALID_FIELD: No such column 'PermissionSetGroupId' on entity 'PermissionSetAssignment'")},
		{match: "FROM PermissionSetAssignment", file: "assignments_nogroup.json"},
		{match: "FROM ObjectPermissions", file: "objectperms.json"},
	}}
	r, err := BuildPermissionsReport(q, "o", alexUser(), "")
	if err != nil {
		t.Fatalf("retry should succeed: %v", err)
	}
	if len(r.Assignments) != 4 || r.Assignments[2].GroupLabel != "" {
		t.Errorf("assignments = %+v", r.Assignments)
	}
	if len(q.seen) < 2 || !strings.Contains(q.seen[0], "MasterLabel") || strings.Contains(q.seen[1], "MasterLabel") {
		t.Errorf("expected retry without group columns, saw %v", q.seen)
	}
}

func TestBuildPermissionsReport_NoAssignments(t *testing.T) {
	q := &fakeSOQL{t: t, routes: []soqlRoute{{match: "FROM PermissionSetAssignment", file: "users_none.json"}}}
	r, err := BuildPermissionsReport(q, "o", alexUser(), "")
	if err != nil {
		t.Fatal(err)
	}
	if r.Assignments == nil || len(r.Assignments) != 0 || r.Objects == nil || len(r.Objects) != 0 || len(r.Errors) != 0 {
		t.Errorf("want empty non-nil sections, got %#v", r)
	}
}

func TestSFCommandError(t *testing.T) {
	exit := &exec.ExitError{ProcessState: &os.ProcessState{}}
	stdoutJSON := []byte(`{"status":1,"name":"INVALID_FIELD","message":"Didn't understand relationship 'PermissionSetGroup' in field path.","exitCode":1}`)
	got := sfCommandError("sf data query", stdoutJSON, exit)
	if got == nil || !strings.Contains(got.Error(), "INVALID_FIELD") || !strings.Contains(got.Error(), "PermissionSetGroup") {
		t.Errorf("stdout JSON error should be surfaced with its name, got %v", got)
	}
	withStderr := &exec.ExitError{ProcessState: &os.ProcessState{}, Stderr: []byte("Warning: a newer version is available\n")}
	if got := sfCommandError("sf data query", stdoutJSON, withStderr); !strings.Contains(got.Error(), "INVALID_FIELD") {
		t.Errorf("stdout JSON must win over stderr warnings, got %v", got)
	}
	if got := sfCommandError("sf data query", nil, withStderr); !strings.Contains(got.Error(), "newer version") {
		t.Errorf("stderr used when stdout has no JSON message, got %v", got)
	}
	if got := sfCommandError("sf data query", nil, errors.New("exec: \"sf\": executable file not found")); !strings.HasPrefix(got.Error(), "sf data query: exec:") {
		t.Errorf("plain error should carry exactly the given label, got %v", got)
	}
}

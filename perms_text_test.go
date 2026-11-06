package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/0xguy07/sf-tui/sf"
)

func fixtureReport(t *testing.T) sf.PermissionsReport {
	t.Helper()
	b, err := os.ReadFile("testdata/perms/report_account.json")
	if err != nil {
		t.Fatal(err)
	}
	var r sf.PermissionsReport
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRenderPermissionsText_Golden(t *testing.T) {
	const golden = "testdata/perms/perms_account.txt"
	got := renderPermissionsText(fixtureReport(t))
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("missing golden (run with UPDATE_GOLDEN=1): %v", err)
	}
	if got != string(want) {
		t.Errorf("differs from golden\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRenderPermissionsText_NoOverrides(t *testing.T) {
	r := fixtureReport(t)
	r.Overrides = sf.Overrides{}
	if strings.Contains(renderPermissionsText(r), "System overrides:") {
		t.Error("overrides section should be omitted when empty")
	}
}

func TestRenderPermissionsText_NoFields(t *testing.T) {
	r := fixtureReport(t)
	r.Fields = []sf.FieldPerm{}
	out := renderPermissionsText(r)
	if !strings.Contains(out, "Fields on Account (0):") || !strings.Contains(out, "no FieldPermissions rows") {
		t.Errorf("empty fields block wrong:\n%s", out)
	}
}

func TestRenderPermissionsText_Inactive(t *testing.T) {
	r := fixtureReport(t)
	r.User.IsActive = false
	first := strings.SplitN(renderPermissionsText(r), "\n", 2)[0]
	if !strings.HasSuffix(first, "[inactive]") {
		t.Errorf("first line = %q", first)
	}
}

func TestRenderPermissionsText_NoANSI(t *testing.T) {
	if strings.Contains(renderPermissionsText(fixtureReport(t)), "\x1b") {
		t.Error("output contains ANSI escapes")
	}
}

func TestRenderPermissionsText_ObjectNotFoundHint(t *testing.T) {
	r := fixtureReport(t)
	r.Object, r.Fields = "Acount", []sf.FieldPerm{}
	out := renderPermissionsText(r)
	if !strings.Contains(out, "no object permissions on Acount either; check the API name") {
		t.Errorf("typo hint missing:\n%s", out)
	}
	r.Object = "Account"
	if strings.Contains(renderPermissionsText(r), "check the API name") {
		t.Error("hint must not appear when the object has permission rows")
	}
}

func TestRenderPermissionsText_NoProfile(t *testing.T) {
	r := fixtureReport(t)
	r.User.ProfileName = ""
	if out := renderPermissionsText(r); !strings.Contains(out, "Profile: (none)") {
		t.Errorf("want 'Profile: (none)':\n%s", out)
	}
}

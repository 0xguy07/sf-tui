package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/0xguy07/sf-tui/sf"
)

func fixtureReportNamed(t *testing.T, name string) sf.PermissionsReport {
	t.Helper()
	b, err := os.ReadFile("testdata/perms/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var r sf.PermissionsReport
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func fixturePermsDiff(t *testing.T) sf.PermissionsDiff {
	t.Helper()
	return sf.DiffPermissions(fixtureReportNamed(t, "report_account.json"), fixtureReportNamed(t, "report_bob.json"))
}

func TestRenderPermissionsDiffText_Golden(t *testing.T) {
	const golden = "testdata/perms/perms_diff.txt"
	got := renderPermissionsDiffText(fixturePermsDiff(t))
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

func TestRenderPermissionsDiffText_Identical(t *testing.T) {
	a := fixtureReportNamed(t, "report_account.json")
	out := renderPermissionsDiffText(sf.DiffPermissions(a, a))
	if !strings.Contains(out, "Object permissions: no differences") || strings.Contains(out, "Assignments only") || strings.Contains(out, "System overrides") {
		t.Errorf("identical users:\n%s", out)
	}
}

func TestRenderPermissionsDiffText_NoANSI(t *testing.T) {
	if strings.Contains(renderPermissionsDiffText(fixturePermsDiff(t)), "\x1b") {
		t.Error("ANSI in output")
	}
}

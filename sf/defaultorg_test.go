package sf

import (
	"os"
	"path/filepath"
	"testing"
)

func cliFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("../testdata/cli", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseTargetOrg(t *testing.T) {
	if got, err := parseTargetOrg(cliFixture(t, "config_target_org.json")); err != nil || got != "dev-sandbox" {
		t.Errorf("set: got %q, %v", got, err)
	}
	if got, err := parseTargetOrg(cliFixture(t, "config_target_org_empty.json")); err != nil || got != "" {
		t.Errorf("empty: got %q, %v; want \"\" and nil", got, err)
	}
	if _, err := parseTargetOrg([]byte("not json")); err == nil {
		t.Error("garbage: want error")
	}
}

func TestDefaultTargetOrg_Seam(t *testing.T) {
	old := sfConfigGet
	defer func() { sfConfigGet = old }()
	sfConfigGet = func() ([]byte, error) { return cliFixture(t, "config_target_org.json"), nil }
	got, err := DefaultTargetOrg()
	if err != nil || got != "dev-sandbox" {
		t.Errorf("got %q, %v", got, err)
	}
}

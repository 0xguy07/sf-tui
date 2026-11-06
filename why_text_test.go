package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/0xguy07/sf-tui/sf"
)

func fixtureTrace(t *testing.T) sf.Trace {
	t.Helper()
	b, err := os.ReadFile("testdata/cli/trace_account.json")
	if err != nil {
		t.Fatal(err)
	}
	var tr sf.Trace
	if err := json.Unmarshal(b, &tr); err != nil {
		t.Fatal(err)
	}
	return tr
}

func TestRenderTraceText_Golden(t *testing.T) {
	const golden = "testdata/cli/why_account.txt"
	got := renderTraceText(fixtureTrace(t))
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
		t.Errorf("text output differs from golden\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRenderTraceText_NoANSI(t *testing.T) {
	if strings.Contains(renderTraceText(fixtureTrace(t)), "\x1b") {
		t.Error("output contains ANSI escapes")
	}
}

func TestRenderTraceText_NoSaves(t *testing.T) {
	tr := fixtureTrace(t)
	tr.Saves = nil
	if got := renderTraceText(tr); !strings.Contains(got, "No tracked field changes found") {
		t.Errorf("want explicit no-saves line, got:\n%s", got)
	}
}

func TestRenderTraceText_ErrorsSection(t *testing.T) {
	tr := fixtureTrace(t)
	tr.Errors = []sf.SectionError{{Section: "automation", Message: "boom"}}
	if got := renderTraceText(tr); !strings.Contains(got, "Errors:\n  automation: boom") {
		t.Errorf("want errors section, got:\n%s", got)
	}
	tr.Errors = nil
	if got := renderTraceText(tr); strings.Contains(got, "Errors:") {
		t.Error("errors section should be omitted when there are none")
	}
}

func TestRenderTraceText_Truncated(t *testing.T) {
	tr := fixtureTrace(t)
	tr.HistoryTruncated = true
	if got := renderTraceText(tr); !strings.Contains(got, "(history truncated") {
		t.Errorf("want truncation note, got:\n%s", got)
	}
}

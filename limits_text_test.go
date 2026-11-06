package main

import (
	"os"
	"strings"
	"testing"

	"github.com/0xguy07/sf-tui/sf"
)

func fixtureLimits(t *testing.T, warn *int) sf.LimitsReport {
	t.Helper()
	b, err := os.ReadFile("testdata/limits/limits.json")
	if err != nil {
		t.Fatal(err)
	}
	ls, err := sf.ParseLimits(b)
	if err != nil {
		t.Fatal(err)
	}
	return sf.BuildLimitsReport(ls, "prod", warn)
}

func intp(n int) *int { return &n }

func TestRenderLimitsText_Golden(t *testing.T) {
	const golden = "testdata/limits/limits_warn90.txt"
	got := renderLimitsText(fixtureLimits(t, intp(90)))
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

func TestRenderLimitsText_NoWarn(t *testing.T) {
	out := renderLimitsText(fixtureLimits(t, nil))
	if strings.Contains(out, "warn at") || strings.Contains(out, "⚠") || strings.Contains(out, "at or above") {
		t.Errorf("no --warn should mean no threshold text:\n%s", out)
	}
	if !strings.Contains(out, "API limits for prod  (5 limits)") {
		t.Errorf("header wrong:\n%s", out)
	}
}

func TestRenderLimitsText_NoANSI(t *testing.T) {
	if strings.Contains(renderLimitsText(fixtureLimits(t, intp(50))), "\x1b") {
		t.Error("ANSI in output")
	}
}

func TestRenderLimitsText_Aligned(t *testing.T) {
	r := fixtureLimits(t, intp(90))
	r.Limits = append(r.Limits, sf.LimitUsage{Name: "PrivateConnectOutboundCalloutHourlyLimitMB", Max: 1000, Remaining: 1000})
	out := renderLimitsText(r)
	col := -1
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "  ") || !strings.Contains(line, "%") {
			continue
		}
		p := strings.Index(line, "%")
		if col == -1 {
			col = p
		} else if p != col {
			t.Errorf("percent column drifts: %d vs %d in %q", p, col, line)
		}
	}
}

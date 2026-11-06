package sf

import (
	"os"
	"reflect"
	"testing"
)

func limitsFixture(t *testing.T) []Limit {
	t.Helper()
	b, err := os.ReadFile("../testdata/limits/limits.json")
	if err != nil {
		t.Fatal(err)
	}
	ls, err := ParseLimits(b)
	if err != nil {
		t.Fatal(err)
	}
	return ls
}

func TestParseLimits(t *testing.T) {
	if ls := limitsFixture(t); len(ls) != 5 || ls[0].Name != "DailyApiRequests" {
		t.Errorf("got %+v", ls)
	}
	if _, err := ParseLimits([]byte("nope")); err == nil {
		t.Error("garbage should error")
	}
}

func TestBuildLimitsReport_SortAndPct(t *testing.T) {
	r := BuildLimitsReport(limitsFixture(t), "prod", nil)
	var names []string
	var pcts []float64
	for _, l := range r.Limits {
		names = append(names, l.Name)
		pcts = append(pcts, l.Pct)
	}
	wantNames := []string{"DailyApiRequests", "DailyBulkApiBatches", "DailyStreamingApiEvents", "DailyAsyncApexExecutions", "PackageVersionCreates"}
	if !reflect.DeepEqual(names, wantNames) {
		t.Errorf("order = %v", names)
	}
	if !reflect.DeepEqual(pcts, []float64{100, 92.5, 10, 0, 0}) {
		t.Errorf("pcts = %v", pcts)
	}
	if r.Org != "prod" || r.Warn != nil || r.Breached == nil || len(r.Breached) != 0 {
		t.Errorf("report = %+v", r)
	}
}

func TestBuildLimitsReport_ClampsUsed(t *testing.T) {
	r := BuildLimitsReport(limitsFixture(t), "prod", nil)
	for _, l := range r.Limits {
		if l.Name == "DailyAsyncApexExecutions" && (l.Used != 0 || l.Pct != 0) {
			t.Errorf("remaining > max should clamp: %+v", l)
		}
	}
}

func TestBuildLimitsReport_Breached(t *testing.T) {
	cases := []struct {
		warn int
		want []string
	}{
		{100, []string{"DailyApiRequests"}},
		{93, []string{"DailyApiRequests"}},
		{92, []string{"DailyApiRequests", "DailyBulkApiBatches"}},
		{10, []string{"DailyApiRequests", "DailyBulkApiBatches", "DailyStreamingApiEvents"}},
	}
	for _, c := range cases {
		w := c.warn
		r := BuildLimitsReport(limitsFixture(t), "prod", &w)
		if !reflect.DeepEqual(r.Breached, c.want) || r.Warn == nil || *r.Warn != c.warn {
			t.Errorf("warn %d: breached = %v, want %v", c.warn, r.Breached, c.want)
		}
	}
}

func TestBuildLimitsReport_StableForEqualPct(t *testing.T) {
	ls := []Limit{{Name: "Zed", Max: 10, Remaining: 5}, {Name: "Alpha", Max: 100, Remaining: 50}}
	r := BuildLimitsReport(ls, "o", nil)
	if r.Limits[0].Name != "Alpha" || r.Limits[1].Name != "Zed" {
		t.Errorf("equal pct should sort by name: %+v", r.Limits)
	}
}

func TestHumanInt(t *testing.T) {
	for n, want := range map[int]string{0: "0", 9999: "9999", 10_000: "10.0K", 1_500_000: "1.5M", 2_000_000_000: "2.0B"} {
		if got := HumanInt(n); got != want {
			t.Errorf("HumanInt(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestBuildLimitsReport_ThresholdUsesRoundedPct(t *testing.T) {
	w := 90
	r := BuildLimitsReport([]Limit{{Name: "X", Max: 10000, Remaining: 1004}}, "o", &w) // 89.96% → 90.0
	if r.Limits[0].Pct != 90.0 || len(r.Breached) != 1 {
		t.Errorf("pct=%v breached=%v; the threshold compares the displayed one-decimal percentage", r.Limits[0].Pct, r.Breached)
	}
}

func TestBuildLimitsReport_NegativeRemaining(t *testing.T) {
	w := 100
	r := BuildLimitsReport([]Limit{{Name: "Soft", Max: 100, Remaining: -20}}, "o", &w)
	if r.Limits[0].Used != 120 || r.Limits[0].Pct != 120.0 || len(r.Breached) != 1 {
		t.Errorf("overage should show >100%% and breach: %+v", r.Limits[0])
	}
}

func TestBuildLimitsReport_ErrorsNeverNull(t *testing.T) {
	r := BuildLimitsReport(nil, "o", nil)
	if r.Errors == nil || len(r.Errors) != 0 || r.Limits == nil {
		t.Errorf("errors/limits must be empty non-nil: %#v", r)
	}
}

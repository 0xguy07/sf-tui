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

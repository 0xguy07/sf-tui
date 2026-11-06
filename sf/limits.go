package sf

import (
	"math"
	"encoding/json"
	"fmt"
	"sort"

	tea "github.com/charmbracelet/bubbletea"
)

type Limit struct {
	Name      string `json:"name"`
	Max       int    `json:"max"`
	Remaining int    `json:"remaining"`
}

func (l Limit) Used() int {
	u := l.Max - l.Remaining
	if u < 0 {
		return 0
	}
	return u
}

func (l Limit) Pct() float64 {
	if l.Max <= 0 {
		return 0
	}
	return float64(l.Used()) / float64(l.Max)
}

type LimitsLoadedMsg struct {
	Limits []Limit
}

type limitsResult struct {
	Status int     `json:"status"`
	Result []Limit `json:"result"`
}

// ParseLimits decodes `sf limits api display --json` output.
func ParseLimits(out []byte) ([]Limit, error) {
	var r limitsResult
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, fmt.Errorf("parse limits: %w", err)
	}
	return r.Result, nil
}

// LimitUsage is one limit with its derived usage, for reports.
type LimitUsage struct {
	Name      string  `json:"name"`
	Max       int     `json:"max"`
	Remaining int     `json:"remaining"`
	Used      int     `json:"used"`
	Pct       float64 `json:"pct"` // 0–100, one decimal
}

// LimitsReport is the non-interactive `sftui limits` answer.
type LimitsReport struct {
	Org      string       `json:"org"`
	Limits   []LimitUsage `json:"limits"`         // sorted by Pct desc, then Name
	Warn     *int         `json:"warn,omitempty"` // the --warn threshold, when given
	Breached []string     `json:"breached"`       // names with Pct >= Warn; never null
	// Errors is always empty today (one sf call, no partial results) and is
	// present so every command's JSON carries the same top-level shape.
	Errors []SectionError `json:"errors"`
}

// BuildLimitsReport derives usage, sorts, and applies an optional threshold.
func BuildLimitsReport(limits []Limit, org string, warn *int) LimitsReport {
	r := LimitsReport{Org: org, Limits: make([]LimitUsage, 0, len(limits)), Warn: warn, Breached: []string{}, Errors: []SectionError{}}
	for _, l := range limits {
		pct := math.Round(l.Pct()*1000) / 10 // one decimal; --warn compares this displayed value
		r.Limits = append(r.Limits, LimitUsage{Name: l.Name, Max: l.Max, Remaining: l.Remaining, Used: l.Used(), Pct: pct})
	}
	sort.SliceStable(r.Limits, func(i, j int) bool {
		if r.Limits[i].Pct != r.Limits[j].Pct {
			return r.Limits[i].Pct > r.Limits[j].Pct
		}
		return r.Limits[i].Name < r.Limits[j].Name
	})
	if warn != nil {
		for _, l := range r.Limits {
			if l.Pct >= float64(*warn) {
				r.Breached = append(r.Breached, l.Name)
			}
		}
	}
	return r
}

// HumanInt formats counts compactly: 9999, 10.0K, 1.5M, 2.0B.
func HumanInt(n int) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", float64(n)/1_000_000_000)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 10_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// FetchLimits runs `sf limits api display` for org and parses the result.
func FetchLimits(org string) ([]Limit, error) {
	out, err := sfCommand("sf limits api display", "limits", "api", "display",
		"--target-org", org,
		"--json",
	).Output()
	if err != nil {
		return nil, sfCommandError("sf limits api display", out, err)
	}
	return ParseLimits(out)
}

func LoadLimits(orgAliasOrUser string) tea.Cmd {
	return func() tea.Msg {
		if orgAliasOrUser == "" {
			return ErrMsg{Err: fmt.Errorf("no org selected")}
		}
		limits, err := FetchLimits(orgAliasOrUser)
		if err != nil {
			return ErrMsg{Err: err}
		}
		// Sort by usage % descending so the interesting ones are at the top.
		sort.SliceStable(limits, func(i, j int) bool {
			if limits[i].Pct() != limits[j].Pct() {
				return limits[i].Pct() > limits[j].Pct()
			}
			return limits[i].Name < limits[j].Name
		})
		return LimitsLoadedMsg{Limits: limits}
	}
}

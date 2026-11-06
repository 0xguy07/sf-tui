package sf

import (
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

func LoadLimits(orgAliasOrUser string) tea.Cmd {
	return func() tea.Msg {
		if orgAliasOrUser == "" {
			return ErrMsg{Err: fmt.Errorf("no org selected")}
		}
		out, err := sfCommand("sf limits api display", "limits", "api", "display",
			"--target-org", orgAliasOrUser,
			"--json",
		).Output()
		if err != nil {
			return ErrMsg{Err: sfCommandError("sf limits api display", out, err)}
		}
		var r limitsResult
		if err := json.Unmarshal(out, &r); err != nil {
			return ErrMsg{Err: fmt.Errorf("parse limits: %w", err)}
		}
		// Sort by usage % descending so the interesting ones are at the top.
		sort.Slice(r.Result, func(i, j int) bool {
			return r.Result[i].Pct() > r.Result[j].Pct()
		})
		return LimitsLoadedMsg{Limits: r.Result}
	}
}

package sf

import (
	"encoding/json"
	"fmt"
	"os/exec"
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
		out, err := exec.Command("sf", "limits", "api", "display",
			"--target-org", orgAliasOrUser,
			"--json",
		).Output()
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
				return ErrMsg{Err: fmt.Errorf("%s", string(ee.Stderr))}
			}
			return ErrMsg{Err: fmt.Errorf("sf limits: %w", err)}
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

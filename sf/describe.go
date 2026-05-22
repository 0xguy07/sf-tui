package sf

import (
	"encoding/json"
	"fmt"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
)

type SObjectSummary struct {
	Name       string `json:"name"`
	Label      string `json:"label"`
	Custom     bool   `json:"custom"`
	Queryable  bool   `json:"queryable"`
	KeyPrefix  string `json:"keyPrefix"`
}

type sobjectListResult struct {
	Status int `json:"status"`
	Result struct {
		SObjects []SObjectSummary `json:"sobjects"`
	} `json:"result"`
}

type PicklistValue struct {
	Value  string `json:"value"`
	Label  string `json:"label"`
	Active bool   `json:"active"`
}

type Field struct {
	Name             string          `json:"name"`
	Label            string          `json:"label"`
	Type             string          `json:"type"`
	Length           int             `json:"length"`
	Nillable         bool            `json:"nillable"`
	Updateable       bool            `json:"updateable"`
	Createable       bool            `json:"createable"`
	Custom           bool            `json:"custom"`
	CalculatedFormula string         `json:"calculatedFormula"`
	InlineHelpText   string          `json:"inlineHelpText"`
	ReferenceTo      []string        `json:"referenceTo"`
	PicklistValues   []PicklistValue `json:"picklistValues"`
}

type Describe struct {
	Name   string  `json:"name"`
	Label  string  `json:"label"`
	Custom bool    `json:"custom"`
	Fields []Field `json:"fields"`
}

type describeResult struct {
	Status int      `json:"status"`
	Result Describe `json:"result"`
}

type SObjectsLoadedMsg struct {
	SObjects []SObjectSummary
}

type DescribeLoadedMsg struct {
	Describe Describe
}

func LoadSObjects(orgAliasOrUser string) tea.Cmd {
	return func() tea.Msg {
		if orgAliasOrUser == "" {
			return ErrMsg{Err: fmt.Errorf("no org selected")}
		}
		out, err := exec.Command("sf", "sobject", "list",
			"--target-org", orgAliasOrUser,
			"--json",
		).Output()
		if err != nil {
			return ErrMsg{Err: fmt.Errorf("sf sobject list: %w", err)}
		}
		// `sf sobject list --json` returns {status, result: [names...]} in some versions,
		// or a wrapped sobjects list in others. Try both.
		var names struct {
			Status int      `json:"status"`
			Result []string `json:"result"`
		}
		if err := json.Unmarshal(out, &names); err == nil && len(names.Result) > 0 {
			summaries := make([]SObjectSummary, 0, len(names.Result))
			for _, n := range names.Result {
				summaries = append(summaries, SObjectSummary{Name: n, Label: n, Queryable: true})
			}
			return SObjectsLoadedMsg{SObjects: summaries}
		}
		var wrapped sobjectListResult
		if err := json.Unmarshal(out, &wrapped); err != nil {
			return ErrMsg{Err: fmt.Errorf("parse sobject list: %w", err)}
		}
		return SObjectsLoadedMsg{SObjects: wrapped.Result.SObjects}
	}
}

func LoadDescribe(orgAliasOrUser, sobject string) tea.Cmd {
	return func() tea.Msg {
		if orgAliasOrUser == "" {
			return ErrMsg{Err: fmt.Errorf("no org selected")}
		}
		out, err := exec.Command("sf", "sobject", "describe",
			"--target-org", orgAliasOrUser,
			"--sobject", sobject,
			"--json",
		).Output()
		if err != nil {
			return ErrMsg{Err: fmt.Errorf("sf sobject describe %s: %w", sobject, err)}
		}
		var r describeResult
		if err := json.Unmarshal(out, &r); err != nil {
			return ErrMsg{Err: fmt.Errorf("parse describe: %w", err)}
		}
		return DescribeLoadedMsg{Describe: r.Result}
	}
}

package sf

import (
	"encoding/json"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type SObjectSummary struct {
	Name      string `json:"name"`
	Label     string `json:"label"`
	Custom    bool   `json:"custom"`
	Queryable bool   `json:"queryable"`
	KeyPrefix string `json:"keyPrefix"`
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
	Name              string          `json:"name"`
	Label             string          `json:"label"`
	Type              string          `json:"type"`
	Length            int             `json:"length"`
	Nillable          bool            `json:"nillable"`
	Updateable        bool            `json:"updateable"`
	Createable        bool            `json:"createable"`
	Custom            bool            `json:"custom"`
	CalculatedFormula string          `json:"calculatedFormula"`
	InlineHelpText    string          `json:"inlineHelpText"`
	ReferenceTo       []string        `json:"referenceTo"`
	PicklistValues    []PicklistValue `json:"picklistValues"`
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
	Cached   bool      // served from the on-disk cache rather than the org
	CachedAt time.Time // when the cached copy was written (zero if live)
}

type DescribeLoadedMsg struct {
	Describe Describe
	Cached   bool
	CachedAt time.Time
}

// LoadSObjects returns the org's sobject list, served from the on-disk cache when
// a fresh copy exists and fetched from the org otherwise. RefreshSObjects forces a
// live fetch, bypassing the cache.
func LoadSObjects(orgAliasOrUser string) tea.Cmd { return loadSObjects(orgAliasOrUser, false) }

func RefreshSObjects(orgAliasOrUser string) tea.Cmd { return loadSObjects(orgAliasOrUser, true) }

func loadSObjects(orgAliasOrUser string, force bool) tea.Cmd {
	return func() tea.Msg {
		if orgAliasOrUser == "" {
			return ErrMsg{Err: fmt.Errorf("no org selected")}
		}
		ss, cached, at, err := fetchSObjects(orgAliasOrUser, force)
		if err != nil {
			return ErrMsg{Err: err}
		}
		return SObjectsLoadedMsg{SObjects: ss, Cached: cached, CachedAt: at}
	}
}

// fetchSObjects returns the org's sobject list, from the on-disk cache when
// fresh (unless force), otherwise from `sf sobject list`, writing the cache.
func fetchSObjects(org string, force bool) ([]SObjectSummary, bool, time.Time, error) {
	if !force {
		if ss, at, ok := readSObjectCache(org); ok && cacheFresh(at) {
			return ss, true, at, nil
		}
	}
	out, err := sfCommand("sf sobject list", "sobject", "list",
		"--target-org", org,
		"--json",
	).Output()
	if err != nil {
		return nil, false, time.Time{}, sfCommandError("sf sobject list", out, err)
	}
	// `sf sobject list --json` returns {status, result: [names...]} in some versions,
	// or a wrapped sobjects list in others. Try both.
	var summaries []SObjectSummary
	var names struct {
		Status int      `json:"status"`
		Result []string `json:"result"`
	}
	if err := json.Unmarshal(out, &names); err == nil && len(names.Result) > 0 {
		summaries = make([]SObjectSummary, 0, len(names.Result))
		for _, n := range names.Result {
			summaries = append(summaries, SObjectSummary{Name: n, Label: n, Queryable: true})
		}
	} else {
		var wrapped sobjectListResult
		if err := json.Unmarshal(out, &wrapped); err != nil {
			return nil, false, time.Time{}, fmt.Errorf("parse sobject list: %w", err)
		}
		summaries = wrapped.Result.SObjects
	}
	writeSObjectCache(org, summaries)
	return summaries, false, time.Time{}, nil
}

// LoadDescribe returns a describe, served from the on-disk cache when fresh and
// fetched from the org otherwise. RefreshDescribe forces a live fetch.
func LoadDescribe(orgAliasOrUser, sobject string) tea.Cmd {
	return loadDescribe(orgAliasOrUser, sobject, false)
}

func RefreshDescribe(orgAliasOrUser, sobject string) tea.Cmd {
	return loadDescribe(orgAliasOrUser, sobject, true)
}

func loadDescribe(orgAliasOrUser, sobject string, force bool) tea.Cmd {
	return func() tea.Msg {
		if orgAliasOrUser == "" {
			return ErrMsg{Err: fmt.Errorf("no org selected")}
		}
		d, cached, at, err := fetchDescribe(orgAliasOrUser, sobject, force)
		if err != nil {
			return ErrMsg{Err: err}
		}
		return DescribeLoadedMsg{Describe: d, Cached: cached, CachedAt: at}
	}
}

// fetchDescribe returns one sobject's describe, from the on-disk cache when
// fresh (unless force), otherwise from `sf sobject describe`, writing the cache.
func fetchDescribe(org, sobject string, force bool) (Describe, bool, time.Time, error) {
	if !force {
		if d, at, ok := readDescribeCache(org, sobject); ok && cacheFresh(at) {
			return d, true, at, nil
		}
	}
	out, err := sfCommand("sf sobject describe "+sobject, "sobject", "describe",
		"--target-org", org,
		"--sobject", sobject,
		"--json",
	).Output()
	if err != nil {
		return Describe{}, false, time.Time{}, sfCommandError("sf sobject describe "+sobject, out, err)
	}
	var r describeResult
	if err := json.Unmarshal(out, &r); err != nil {
		return Describe{}, false, time.Time{}, fmt.Errorf("parse describe: %w", err)
	}
	writeDescribeCache(org, r.Result)
	return r.Result, false, time.Time{}, nil
}

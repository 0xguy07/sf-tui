package sf

import (
	"encoding/json"
	"fmt"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
)

type OrgsLoadedMsg struct {
	Orgs []Org
}

type QueryDoneMsg struct {
	Rows    []QueryRecord
	Columns []string
	Total   int
	SObject string // FROM target, used by record editor
}

type ErrMsg struct {
	Err error
}

func (e ErrMsg) Error() string { return e.Err.Error() }

func LoadOrgs() tea.Cmd {
	return func() tea.Msg {
		out, err := exec.Command("sf", "org", "list", "--json").Output()
		if err != nil {
			return ErrMsg{Err: fmt.Errorf("sf org list: %w", err)}
		}
		all, err := parseOrgList(out)
		if err != nil {
			return ErrMsg{Err: err}
		}
		return OrgsLoadedMsg{Orgs: all}
	}
}

func parseOrgList(out []byte) ([]Org, error) {
	var r OrgListResult
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, fmt.Errorf("parse org list: %w", err)
	}
	all := append([]Org{}, r.Result.NonScratchOrgs...)
	for _, o := range r.Result.ScratchOrgs {
		o.IsScratch = true
		all = append(all, o)
	}
	all = append(all, r.Result.Other...)
	return all, nil
}

func RunQuery(orgAliasOrUser, soql string) tea.Cmd {
	return runQueryArgs(orgAliasOrUser, soql, false)
}

func RunQueryTooling(orgAliasOrUser, soql string) tea.Cmd {
	return runQueryArgs(orgAliasOrUser, soql, true)
}

func runQueryArgs(orgAliasOrUser, soql string, tooling bool) tea.Cmd {
	return func() tea.Msg {
		if orgAliasOrUser == "" {
			return ErrMsg{Err: fmt.Errorf("no org selected")}
		}
		args := []string{"data", "query",
			"--target-org", orgAliasOrUser,
			"--query", soql,
			"--json",
		}
		if tooling {
			args = append(args, "--use-tooling-api")
		}
		cmd := exec.Command("sf", args...)
		out, err := cmd.Output()
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
				return ErrMsg{Err: fmt.Errorf("%s", string(ee.Stderr))}
			}
			var r QueryResult
			if jerr := json.Unmarshal(out, &r); jerr == nil && r.Message != "" {
				return ErrMsg{Err: fmt.Errorf("%s", r.Message)}
			}
			return ErrMsg{Err: fmt.Errorf("sf data query: %w", err)}
		}
		var r QueryResult
		if err := json.Unmarshal(out, &r); err != nil {
			return ErrMsg{Err: fmt.Errorf("parse query result: %w", err)}
		}
		var cols []string
		var sobject string
		if info, ok := ParseSelect(soql); ok && len(info.Columns) > 0 {
			cols = info.Columns
			sobject = info.SObject
		} else {
			cols = deriveColumns(r.Result.Records)
		}
		return QueryDoneMsg{
			Rows:    r.Result.Records,
			Columns: cols,
			Total:   r.Result.TotalSize,
			SObject: sobject,
		}
	}
}

func deriveColumns(rows []QueryRecord) []string {
	if len(rows) == 0 {
		return nil
	}
	seen := map[string]bool{}
	cols := []string{}
	for _, r := range rows {
		for k := range r {
			if k == "attributes" || seen[k] {
				continue
			}
			seen[k] = true
			cols = append(cols, k)
		}
	}
	return cols
}

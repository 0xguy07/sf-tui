package sf

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type RecordUpdatedMsg struct {
	ID      string
	SObject string
	Updates map[string]string
}

// FormatUpdateValues turns a map of field→value into the space-separated
// key='value' format that `sf data update record --values` expects. Single
// quotes inside the value are escaped as '\''. Empty values become "" so
// they clear the field.
func FormatUpdateValues(updates map[string]string) string {
	keys := make([]string, 0, len(updates))
	for k := range updates {
		keys = append(keys, k)
	}
	// Stable order makes the assembled command deterministic — easier to log
	// and to test.
	sortStrings(keys)

	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, escapeValue(updates[k])))
	}
	return strings.Join(parts, " ")
}

func escapeValue(v string) string {
	if v == "" {
		return "\"\""
	}
	// Wrap in single quotes; embedded single quotes use the shell-style
	// concatenation '\'' which the sf CLI accepts via the --values arg.
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// UpdateRecord runs `sf data update record` with the given field changes.
func UpdateRecord(orgAliasOrUser, sobject, id string, updates map[string]string) tea.Cmd {
	return func() tea.Msg {
		if orgAliasOrUser == "" {
			return ErrMsg{Err: fmt.Errorf("no org selected")}
		}
		if sobject == "" || id == "" {
			return ErrMsg{Err: fmt.Errorf("missing sobject or record id")}
		}
		if len(updates) == 0 {
			return ErrMsg{Err: fmt.Errorf("no changes to save")}
		}
		args := []string{
			"data", "update", "record",
			"--target-org", orgAliasOrUser,
			"--sobject", sobject,
			"--record-id", id,
			"--values", FormatUpdateValues(updates),
			"--json",
		}
		cmd := exec.Command("sf", args...)
		out, err := cmd.Output()
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
				return ErrMsg{Err: fmt.Errorf("%s", string(ee.Stderr))}
			}
			// Some sf errors come back as JSON on stdout, not stderr.
			var r struct {
				Message string `json:"message"`
			}
			if jerr := json.Unmarshal(out, &r); jerr == nil && r.Message != "" {
				return ErrMsg{Err: fmt.Errorf("%s", r.Message)}
			}
			return ErrMsg{Err: fmt.Errorf("sf data update record: %w", err)}
		}
		return RecordUpdatedMsg{ID: id, SObject: sobject, Updates: updates}
	}
}

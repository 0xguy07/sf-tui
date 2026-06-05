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

// FormatUpdateValues turns a map of field->value into the space-separated
// key=value format that `sf data update record --values` expects, choosing a
// quote wrapper per value (see escapeValue). Empty values become "" so they
// clear the field. Keys are emitted in sorted order for deterministic output.
func FormatUpdateValues(updates map[string]string) string {
	keys := make([]string, 0, len(updates))
	for k := range updates {
		keys = append(keys, k)
	}
	// Stable order makes the assembled command deterministic: easier to log
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
		return `""`
	}
	// The sf CLI's --values parser (stringToDictionary in plugin-data) has NO
	// escape mechanism: it removes quote characters by toggling in/out state as
	// it scans. So you cannot escape a quote; you can only pick a wrapper that
	// does not appear inside the value. Double-quote wrapping preserves spaces
	// AND apostrophes (the common case in Salesforce data: O'Brien, Macy's), so
	// we prefer it, and fall back to single quotes only when the value itself
	// contains a double quote.
	if !strings.Contains(v, `"`) {
		return `"` + v + `"`
	}
	if !strings.Contains(v, "'") {
		return "'" + v + "'"
	}
	// Value contains BOTH a single and a double quote: unrepresentable via the
	// --values flag (no escaping exists). Double-quote wrap is the least-bad
	// option; the embedded double quotes will be dropped by the parser.
	return `"` + v + `"`
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

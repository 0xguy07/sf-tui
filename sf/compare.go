package sf

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// SchemaLoadedMsg delivers the sobject list for one side of an org compare.
// Side is "A" or "B".
type SchemaLoadedMsg struct {
	Side     string
	Org      string
	SObjects []SObjectSummary
}

// LoadOrgSchema fetches the sobject list for an org and tags it with side.
// Reuses sf sobject list (no per-object describes) so the diff is fast.
func LoadOrgSchema(orgAliasOrUser, side string) tea.Cmd {
	return func() tea.Msg {
		if orgAliasOrUser == "" {
			return ErrMsg{Err: fmt.Errorf("no org selected")}
		}
		raw := LoadSObjects(orgAliasOrUser)()
		switch v := raw.(type) {
		case SObjectsLoadedMsg:
			return SchemaLoadedMsg{Side: side, Org: orgAliasOrUser, SObjects: v.SObjects}
		case ErrMsg:
			return v
		default:
			return ErrMsg{Err: fmt.Errorf("unexpected message type from LoadSObjects")}
		}
	}
}

// FieldDiffLoadedMsg carries the field-level describe results for one sobject
// across both sides of a compare. Either A or B may be nil if the sobject only
// exists on one side or the describe failed there.
type FieldDiffLoadedMsg struct {
	SObject string
	AOrg    string
	BOrg    string
	A       *Describe
	B       *Describe
	AErr    string
	BErr    string
}

// LoadFieldDiff fetches the describe for sobject from both A and B sides in
// parallel-ish (sequential, but the sf CLI is the slow part either way) and
// returns a single message with both results. An empty org string means "skip
// that side" — useful when we already know the sobject doesn't exist there.
func LoadFieldDiff(aOrg, bOrg, sobject string) tea.Cmd {
	return func() tea.Msg {
		msg := FieldDiffLoadedMsg{SObject: sobject, AOrg: aOrg, BOrg: bOrg}
		if aOrg != "" {
			raw := LoadDescribe(aOrg, sobject)()
			switch v := raw.(type) {
			case DescribeLoadedMsg:
				d := v.Describe
				msg.A = &d
			case ErrMsg:
				msg.AErr = v.Error()
			}
		}
		if bOrg != "" {
			raw := LoadDescribe(bOrg, sobject)()
			switch v := raw.(type) {
			case DescribeLoadedMsg:
				d := v.Describe
				msg.B = &d
			case ErrMsg:
				msg.BErr = v.Error()
			}
		}
		return msg
	}
}

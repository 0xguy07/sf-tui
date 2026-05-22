package panes

import (
	"regexp"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

type Query struct {
	Area textarea.Model
}

var tailTokenRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z0-9_]*)*$`)

// ReplaceTailToken replaces the trailing identifier (optionally dotted) in the
// query text with `insert`. If there is no trailing token, the text is appended.
// Returns the new full value.
func (q *Query) ReplaceTailToken(insert string) string {
	v := q.Area.Value()
	loc := tailTokenRe.FindStringIndex(v)
	var newVal string
	if loc == nil {
		newVal = v + insert
	} else {
		// If the existing tail is a dotted path, keep everything up to the last dot.
		tail := v[loc[0]:loc[1]]
		lastDot := -1
		for i := len(tail) - 1; i >= 0; i-- {
			if tail[i] == '.' {
				lastDot = i
				break
			}
		}
		if lastDot >= 0 {
			newVal = v[:loc[0]+lastDot+1] + insert
		} else {
			newVal = v[:loc[0]] + insert
		}
	}
	q.Area.SetValue(newVal)
	return newVal
}

func NewQuery(width, height int) Query {
	ta := textarea.New()
	ta.Placeholder = "SELECT Id, Subject FROM Case LIMIT 10"
	ta.SetWidth(width)
	ta.SetHeight(height)
	ta.ShowLineNumbers = false
	ta.CharLimit = 8000
	return Query{Area: ta}
}

func (q *Query) Focus() tea.Cmd   { return q.Area.Focus() }
func (q *Query) Blur()            { q.Area.Blur() }
func (q Query) Focused() bool     { return q.Area.Focused() }
func (q Query) Value() string     { return q.Area.Value() }
func (q *Query) SetValue(s string) { q.Area.SetValue(s) }

func (q Query) Update(msg tea.Msg) (Query, tea.Cmd) {
	var cmd tea.Cmd
	q.Area, cmd = q.Area.Update(msg)
	return q, cmd
}

func (q Query) View() string { return q.Area.View() }

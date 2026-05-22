package panes

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/0xguy07/sf-tui/sf"
)

type Results struct {
	Table   table.Model
	Total   int
	Width   int
	Height  int
	Cols    []string
	Records []sf.QueryRecord
}

func NewResults(width, height int) Results {
	t := table.New(
		table.WithFocused(false),
		table.WithHeight(height),
	)
	// Bind ←/→ to page nav for parity with the lists.
	t.KeyMap.PageUp = key.NewBinding(key.WithKeys("left", "b", "pgup"), key.WithHelp("←/b/pgup", "prev page"))
	t.KeyMap.PageDown = key.NewBinding(key.WithKeys("right", "f", "pgdown"), key.WithHelp("→/f/pgdn", "next page"))
	return Results{Table: t, Width: width, Height: height}
}

func (r *Results) SetSize(width, height int) {
	r.Width = width
	r.Height = height
	r.Table.SetHeight(height)
	r.Table.SetWidth(width)
}

func (r *Results) SetRows(cols []string, rows []sf.QueryRecord, total int) {
	r.Cols = cols
	r.Records = rows
	r.Total = total

	if len(cols) == 0 {
		r.Table.SetColumns(nil)
		r.Table.SetRows(nil)
		return
	}

	colWidth := r.Width / len(cols)
	if colWidth < 10 {
		colWidth = 10
	}
	cs := make([]table.Column, 0, len(cols))
	for _, c := range cols {
		cs = append(cs, table.Column{Title: c, Width: colWidth})
	}
	r.Table.SetColumns(cs)

	rs := make([]table.Row, 0, len(rows))
	for _, rec := range rows {
		row := make(table.Row, 0, len(cols))
		for _, c := range cols {
			row = append(row, formatCell(sf.ResolvePath(rec, c), colWidth))
		}
		rs = append(rs, row)
	}
	r.Table.SetRows(rs)
}

// SelectedRecord returns the QueryRecord for the highlighted row, or nil.
func (r Results) SelectedRecord() sf.QueryRecord {
	if len(r.Records) == 0 {
		return nil
	}
	idx := r.Table.Cursor()
	if idx < 0 || idx >= len(r.Records) {
		return nil
	}
	return r.Records[idx]
}

func formatCell(v any, width int) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return clip(x, width)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		// Render integers without trailing .0
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%g", x)
	case map[string]any:
		// Salesforce often returns nested objects with just an "attributes"
		// envelope when no fields were selected from the relationship.
		// Show a hint rather than raw map[].
		return clip("{relationship}", width)
	case []any:
		return clip(fmt.Sprintf("[%d]", len(x)), width)
	default:
		return clip(fmt.Sprintf("%v", v), width)
	}
}

func clip(s string, width int) string {
	if width <= 1 || len(s) <= width {
		return s
	}
	// Strip newlines so cells don't break the table layout.
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= width {
		return s
	}
	return s[:width-1] + "…"
}

func (r Results) Update(msg tea.Msg) (Results, tea.Cmd) {
	var cmd tea.Cmd
	r.Table, cmd = r.Table.Update(msg)
	return r, cmd
}

func (r *Results) Focus()      { r.Table.Focus() }
func (r *Results) Blur()       { r.Table.Blur() }
func (r Results) View() string { return r.Table.View() }

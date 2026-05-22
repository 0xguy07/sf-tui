package panes

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/0xguy07/sf-tui/sf"
)

// fieldRow holds the state of a single editable field while the editor is open.
type fieldRow struct {
	Field    sf.Field
	Original string
	Current  string
	Dirty    bool
}

func (f fieldRow) Title() string {
	marker := "  "
	if f.Dirty {
		marker = "● "
	}
	return marker + f.Field.Name
}

func (f fieldRow) Description() string {
	val := f.Current
	if val == "" {
		val = lipgloss.NewStyle().Faint(true).Render("(empty)")
	}
	if len(val) > 40 {
		val = val[:37] + "…"
	}
	return fmt.Sprintf("%s · %s", f.Field.Type, val)
}

func (f fieldRow) FilterValue() string { return f.Field.Name + " " + f.Field.Label }

type editFocus int

const (
	editFocusList editFocus = iota
	editFocusValue
)

// Record is a modal pane that edits a single record's updateable fields.
type Record struct {
	List     list.Model
	Input    textinput.Model
	Hint     viewport.Model
	SObject  string
	RecordID string
	Visible  bool
	Width    int
	Height   int

	rows  []fieldRow
	focus editFocus
}

func NewRecord(width, height int) Record {
	l := list.New(nil, list.NewDefaultDelegate(), width/2, height-4)
	l.Title = "Fields"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)

	ti := textinput.New()
	ti.Width = width / 2
	ti.CharLimit = 4000

	v := viewport.New(width/2, height-4)

	return Record{
		List:   l,
		Input:  ti,
		Hint:   v,
		Width:  width,
		Height: height,
		focus:  editFocusList,
	}
}

func (r *Record) SetSize(width, height int) {
	r.Width = width
	r.Height = height
	half := width / 2
	r.List.SetSize(half, height-4)
	r.Input.Width = half - 4
	r.Hint.Width = half
	r.Hint.Height = height - 4
}

// Open populates the editor from a describe + the current record's values.
// Only updateable, non-formula fields are shown.
func (r *Record) Open(sobject string, d sf.Describe, rec sf.QueryRecord) {
	r.SObject = sobject
	r.RecordID = stringValue(sf.ResolvePath(rec, "Id"))
	r.rows = nil
	for _, f := range d.Fields {
		if !f.Updateable || f.CalculatedFormula != "" {
			continue
		}
		val := stringValue(sf.ResolvePath(rec, f.Name))
		r.rows = append(r.rows, fieldRow{
			Field:    f,
			Original: val,
			Current:  val,
		})
	}
	r.refreshList()
	r.focus = editFocusList
	r.Visible = true
	r.syncInputToSelection()
	r.renderHint()
}

func (r *Record) Close() {
	r.Visible = false
	r.rows = nil
	r.RecordID = ""
}

func (r *Record) refreshList() {
	items := make([]list.Item, 0, len(r.rows))
	for _, f := range r.rows {
		items = append(items, f)
	}
	r.List.SetItems(items)
}

func (r *Record) selectedIndex() int {
	idx := r.List.Index()
	if idx < 0 || idx >= len(r.rows) {
		return -1
	}
	return idx
}

func (r *Record) syncInputToSelection() {
	idx := r.selectedIndex()
	if idx < 0 {
		r.Input.SetValue("")
		return
	}
	r.Input.SetValue(r.rows[idx].Current)
	r.Input.CursorEnd()
}

// commitInputToSelection writes the input box value back into the field row,
// updating dirty state.
func (r *Record) commitInputToSelection() {
	idx := r.selectedIndex()
	if idx < 0 {
		return
	}
	v := r.Input.Value()
	r.rows[idx].Current = v
	r.rows[idx].Dirty = v != r.rows[idx].Original
	r.refreshList()
	r.List.Select(idx) // keep cursor pinned
}

// Dirty returns true if any field has been changed.
func (r Record) Dirty() bool {
	for _, f := range r.rows {
		if f.Dirty {
			return true
		}
	}
	return false
}

// Updates returns the map of field→new-value for everything dirty.
func (r Record) Updates() map[string]string {
	out := map[string]string{}
	for _, f := range r.rows {
		if f.Dirty {
			out[f.Field.Name] = f.Current
		}
	}
	return out
}

// Apply marks the editor's current state as the new clean baseline. Called
// after a successful save.
func (r *Record) Apply() {
	for i := range r.rows {
		r.rows[i].Original = r.rows[i].Current
		r.rows[i].Dirty = false
	}
	r.refreshList()
}

func (r *Record) FocusValue() {
	r.focus = editFocusValue
	_ = r.Input.Focus()
}

func (r *Record) FocusList() {
	r.focus = editFocusList
	r.Input.Blur()
}

func (r Record) ValueFocused() bool { return r.focus == editFocusValue }

func (r Record) Update(msg tea.Msg) (Record, tea.Cmd) {
	var cmd tea.Cmd
	if r.focus == editFocusValue {
		r.Input, cmd = r.Input.Update(msg)
		r.commitInputToSelection()
	} else {
		prevIdx := r.selectedIndex()
		r.List, cmd = r.List.Update(msg)
		if r.selectedIndex() != prevIdx {
			r.syncInputToSelection()
			r.renderHint()
		}
	}
	return r, cmd
}

func (r *Record) renderHint() {
	idx := r.selectedIndex()
	if idx < 0 {
		r.Hint.SetContent("")
		return
	}
	f := r.rows[idx].Field
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(f.Name))
	if f.Label != "" && f.Label != f.Name {
		b.WriteString("  " + lipgloss.NewStyle().Faint(true).Render(f.Label))
	}
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(typeDescription(f)) + "\n\n")

	if f.InlineHelpText != "" {
		b.WriteString(f.InlineHelpText + "\n\n")
	}
	if len(f.PicklistValues) > 0 {
		b.WriteString(lipgloss.NewStyle().Bold(true).Render("Picklist values:") + "\n")
		for _, p := range f.PicklistValues {
			if !p.Active {
				continue
			}
			b.WriteString("  " + p.Value + "\n")
		}
	}
	r.Hint.SetContent(b.String())
	r.Hint.GotoTop()
}

func typeDescription(f sf.Field) string {
	t := f.Type
	if f.Length > 0 && (f.Type == "string" || f.Type == "textarea") {
		t = fmt.Sprintf("%s(%d)", f.Type, f.Length)
	}
	if f.Type == "reference" && len(f.ReferenceTo) > 0 {
		t = "reference→" + strings.Join(f.ReferenceTo, ",")
	}
	flags := []string{}
	if !f.Nillable {
		flags = append(flags, "required")
	}
	if f.Custom {
		flags = append(flags, "custom")
	}
	if len(flags) > 0 {
		t += "  [" + strings.Join(flags, ",") + "]"
	}
	return t
}

func stringValue(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%g", x)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func (r Record) View() string {
	if !r.Visible {
		return ""
	}
	listStyle := idleBorder
	valueStyle := idleBorder
	if r.focus == editFocusList {
		listStyle = activeBorder
	} else {
		valueStyle = activeBorder
	}

	var label string
	if idx := r.selectedIndex(); idx >= 0 {
		label = r.rows[idx].Field.Name
	}
	editorBox := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Bold(true).Render("Edit: "+label),
		r.Input.View(),
		"",
		r.Hint.View(),
	)

	body := lipgloss.JoinHorizontal(lipgloss.Top,
		listStyle.Render(r.List.View()),
		valueStyle.Render(editorBox),
	)

	header := lipgloss.NewStyle().Bold(true).Render(
		fmt.Sprintf("Record editor — %s · %s", r.SObject, r.RecordID))
	return lipgloss.JoinVertical(lipgloss.Left, header, body)
}

package panes

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/0xguy07/sf-tui/sf"
)

type diffItem struct {
	name   string
	state  string // "only-a", "only-b", "both", "different"
	custom bool
}

func (d diffItem) Title() string {
	return diffGlyph(d.state) + " " + d.name
}
func (d diffItem) Description() string {
	if d.custom {
		return "(custom)"
	}
	return ""
}
func (d diffItem) FilterValue() string { return d.name }

type Compare struct {
	List     list.Model
	Viewport viewport.Model
	width    int
	height   int

	aOrg string
	bOrg string
	aObj map[string]sf.SObjectSummary
	bObj map[string]sf.SObjectSummary

	// fieldView is the sobject whose field-level diff is currently displayed
	// in the viewport (cleared when user navigates back to the summary).
	fieldView string
}

func NewCompare(width, height int) Compare {
	l := list.New(nil, list.NewDefaultDelegate(), width/2, height)
	l.Title = "Schema diff"
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)
	v := viewport.New(width/2, height)
	v.SetContent(lipgloss.NewStyle().Faint(true).Render(
		"Org compare: pick org A in the sidebar, ctrl+r to load.\n" +
			"Then pick org B and ctrl+r again. 'c' clears."))
	return Compare{
		List: l, Viewport: v, width: width, height: height,
		aObj: map[string]sf.SObjectSummary{},
		bObj: map[string]sf.SObjectSummary{},
	}
}

func (c *Compare) SetSize(width, height int) {
	c.width = width
	c.height = height
	listW := width / 2
	if listW < 30 {
		listW = 30
	}
	detailW := width - listW - 4
	if detailW < 20 {
		detailW = 20
	}
	c.List.SetSize(listW, height)
	c.Viewport.Width = detailW
	c.Viewport.Height = height
}

// NextSide reports which side ctrl+r should load next: "A" if A is empty,
// "B" if A is loaded but B is not, "" if both are already loaded.
func (c Compare) NextSide() string {
	if c.aOrg == "" {
		return "A"
	}
	if c.bOrg == "" {
		return "B"
	}
	return ""
}

// AOrg / BOrg expose the loaded org aliases so callers can dispatch describes.
func (c Compare) AOrg() string { return c.aOrg }
func (c Compare) BOrg() string { return c.bOrg }

// BothLoaded is true once schemas for A and B are both in memory.
func (c Compare) BothLoaded() bool { return c.aOrg != "" && c.bOrg != "" }

// SelectedSObject returns the sobject name currently highlighted in the diff
// list, or "" if nothing is selected.
func (c Compare) SelectedSObject() string {
	it, ok := c.List.SelectedItem().(diffItem)
	if !ok {
		return ""
	}
	return it.name
}

// SelectedSides reports which sides the selected sobject exists in. Returns
// inA, inB. Returns false, false if no selection.
func (c Compare) SelectedSides() (bool, bool) {
	name := c.SelectedSObject()
	if name == "" {
		return false, false
	}
	_, inA := c.aObj[name]
	_, inB := c.bObj[name]
	return inA, inB
}

func (c *Compare) SetSchema(side, org string, objs []sf.SObjectSummary) {
	m := map[string]sf.SObjectSummary{}
	for _, o := range objs {
		m[o.Name] = o
	}
	switch side {
	case "A":
		c.aOrg = org
		c.aObj = m
	case "B":
		c.bOrg = org
		c.bObj = m
	}
	c.fieldView = ""
	c.refresh()
}

func (c *Compare) Clear() {
	c.aOrg = ""
	c.bOrg = ""
	c.aObj = map[string]sf.SObjectSummary{}
	c.bObj = map[string]sf.SObjectSummary{}
	c.fieldView = ""
	c.List.SetItems(nil)
	c.Viewport.SetContent(lipgloss.NewStyle().Faint(true).Render(
		"Cleared. ctrl+r to load A side."))
}

// ShowSummary returns the viewport to the org-level summary (the default view).
// Used when the user backs out of a field-level diff.
func (c *Compare) ShowSummary() {
	c.fieldView = ""
	c.refresh()
}

// InFieldView reports whether the viewport is currently showing a field-level
// diff (vs. the summary).
func (c Compare) InFieldView() bool { return c.fieldView != "" }

func (c *Compare) SetStatus(s string) {
	c.Viewport.SetContent(lipgloss.NewStyle().Faint(true).Render(s))
}

// SetFieldDiff renders the field-level diff for one sobject into the viewport.
func (c *Compare) SetFieldDiff(msg sf.FieldDiffLoadedMsg) {
	c.fieldView = msg.SObject
	c.Viewport.SetContent(renderFieldDiff(msg, c.aOrg, c.bOrg))
	c.Viewport.GotoTop()
}

func (c *Compare) refresh() {
	if c.aOrg == "" || c.bOrg == "" {
		var b strings.Builder
		fmt.Fprintln(&b, cmpHeader.Render("Compare"))
		if c.aOrg != "" {
			fmt.Fprintf(&b, "  A: %s (%d sobjects)\n", c.aOrg, len(c.aObj))
		} else {
			fmt.Fprintln(&b, cmpDim.Render("  A: not loaded"))
		}
		if c.bOrg != "" {
			fmt.Fprintf(&b, "  B: %s (%d sobjects)\n", c.bOrg, len(c.bObj))
		} else {
			fmt.Fprintln(&b, cmpDim.Render("  B: not loaded"))
		}
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, cmpDim.Render("ctrl+r loads the next side."))
		c.Viewport.SetContent(b.String())
		return
	}

	names := map[string]bool{}
	for n := range c.aObj {
		names[n] = true
	}
	for n := range c.bObj {
		names[n] = true
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)

	var onlyA, onlyB, both int
	items := make([]list.Item, 0, len(sorted))
	for _, n := range sorted {
		_, inA := c.aObj[n]
		bObj, inB := c.bObj[n]
		state := ""
		switch {
		case inA && !inB:
			state = "only-a"
			onlyA++
		case !inA && inB:
			state = "only-b"
			onlyB++
		default:
			state = "both"
			both++
		}
		custom := false
		if inA {
			custom = c.aObj[n].Custom
		} else {
			custom = bObj.Custom
		}
		items = append(items, diffItem{name: n, state: state, custom: custom})
	}
	c.List.SetItems(items)

	var b strings.Builder
	fmt.Fprintln(&b, cmpHeader.Render("Compare"))
	fmt.Fprintf(&b, "  A: %s (%d)\n", c.aOrg, len(c.aObj))
	fmt.Fprintf(&b, "  B: %s (%d)\n", c.bOrg, len(c.bObj))
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "  %s only in A:  %d\n", cmpA.Render("◀"), onlyA)
	fmt.Fprintf(&b, "  %s only in B:  %d\n", cmpB.Render("▶"), onlyB)
	fmt.Fprintf(&b, "  %s in both:    %d\n", cmpDim.Render("="), both)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, cmpDim.Render("enter on a row → field-level diff (both sides). esc returns here."))
	c.Viewport.SetContent(b.String())
}

var (
	cmpHeader = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	cmpDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	cmpA      = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	cmpB      = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	cmpBoth   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	cmpAdd    = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	cmpDel    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	cmpChg    = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	cmpErr    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

func diffGlyph(state string) string {
	switch state {
	case "only-a":
		return cmpA.Render("◀")
	case "only-b":
		return cmpB.Render("▶")
	case "both":
		return cmpBoth.Render("=")
	}
	return " "
}

// renderFieldDiff produces the field-level diff view for one sobject. It walks
// the union of fields on both sides and groups them into added (only B),
// removed (only A), and changed (different attributes).
func renderFieldDiff(msg sf.FieldDiffLoadedMsg, aOrg, bOrg string) string {
	var b strings.Builder
	fmt.Fprintln(&b, cmpHeader.Render("Field diff: "+msg.SObject))
	fmt.Fprintf(&b, "  A: %s\n", aOrg)
	fmt.Fprintf(&b, "  B: %s\n", bOrg)
	fmt.Fprintln(&b)

	if msg.AErr != "" {
		fmt.Fprintln(&b, cmpErr.Render("A describe failed: "+msg.AErr))
	}
	if msg.BErr != "" {
		fmt.Fprintln(&b, cmpErr.Render("B describe failed: "+msg.BErr))
	}

	aFields := map[string]sf.Field{}
	bFields := map[string]sf.Field{}
	if msg.A != nil {
		for _, f := range msg.A.Fields {
			aFields[f.Name] = f
		}
	}
	if msg.B != nil {
		for _, f := range msg.B.Fields {
			bFields[f.Name] = f
		}
	}

	all := map[string]bool{}
	for n := range aFields {
		all[n] = true
	}
	for n := range bFields {
		all[n] = true
	}
	names := make([]string, 0, len(all))
	for n := range all {
		names = append(names, n)
	}
	sort.Strings(names)

	type changedRow struct {
		name string
		why  []string
	}

	var onlyA, onlyB []sf.Field
	var changed []changedRow
	var same int
	for _, n := range names {
		af, inA := aFields[n]
		bf, inB := bFields[n]
		switch {
		case inA && !inB:
			onlyA = append(onlyA, af)
		case !inA && inB:
			onlyB = append(onlyB, bf)
		default:
			diffs := fieldAttrDiff(af, bf)
			if len(diffs) == 0 {
				same++
			} else {
				changed = append(changed, changedRow{name: n, why: diffs})
			}
		}
	}

	fmt.Fprintf(&b, "  %s  %d only in A     %s  %d only in B     %s  %d changed     %s  %d same\n",
		cmpDel.Render("-"), len(onlyA),
		cmpAdd.Render("+"), len(onlyB),
		cmpChg.Render("~"), len(changed),
		cmpDim.Render("="), same)
	fmt.Fprintln(&b)

	if len(onlyA) > 0 {
		fmt.Fprintln(&b, cmpDel.Render("Only in A (would be deleted on B):"))
		for _, f := range onlyA {
			fmt.Fprintf(&b, "  %s %s  %s\n",
				cmpDel.Render("-"), f.Name, cmpDim.Render(fieldTypeLabel(f)))
		}
		fmt.Fprintln(&b)
	}
	if len(onlyB) > 0 {
		fmt.Fprintln(&b, cmpAdd.Render("Only in B (would be added to A):"))
		for _, f := range onlyB {
			fmt.Fprintf(&b, "  %s %s  %s\n",
				cmpAdd.Render("+"), f.Name, cmpDim.Render(fieldTypeLabel(f)))
		}
		fmt.Fprintln(&b)
	}
	if len(changed) > 0 {
		fmt.Fprintln(&b, cmpChg.Render("Changed (different attributes):"))
		for _, c := range changed {
			fmt.Fprintf(&b, "  %s %s\n", cmpChg.Render("~"), c.name)
			for _, w := range c.why {
				fmt.Fprintf(&b, "      %s\n", cmpDim.Render(w))
			}
		}
		fmt.Fprintln(&b)
	}
	if len(onlyA) == 0 && len(onlyB) == 0 && len(changed) == 0 && msg.A != nil && msg.B != nil {
		fmt.Fprintln(&b, cmpBoth.Render("No field-level differences."))
	}
	return b.String()
}

func fieldTypeLabel(f sf.Field) string {
	t := f.Type
	if f.Type == "reference" && len(f.ReferenceTo) > 0 {
		t = "reference→" + strings.Join(f.ReferenceTo, ",")
	}
	if f.Length > 0 && (f.Type == "string" || f.Type == "textarea") {
		t = fmt.Sprintf("%s(%d)", f.Type, f.Length)
	}
	tags := []string{}
	if !f.Nillable {
		tags = append(tags, "required")
	}
	if f.Custom {
		tags = append(tags, "custom")
	}
	if !f.Updateable {
		tags = append(tags, "read-only")
	}
	if len(tags) > 0 {
		t += " [" + strings.Join(tags, ",") + "]"
	}
	return t
}

// fieldAttrDiff returns the human-readable list of attributes that differ
// between the two sides of the same field. Empty slice means identical.
func fieldAttrDiff(a, b sf.Field) []string {
	var diffs []string
	if a.Type != b.Type {
		diffs = append(diffs, fmt.Sprintf("type: %s → %s", a.Type, b.Type))
	}
	if a.Length != b.Length {
		diffs = append(diffs, fmt.Sprintf("length: %d → %d", a.Length, b.Length))
	}
	if a.Nillable != b.Nillable {
		diffs = append(diffs, fmt.Sprintf("required: %v → %v", !a.Nillable, !b.Nillable))
	}
	if a.Updateable != b.Updateable {
		diffs = append(diffs, fmt.Sprintf("updateable: %v → %v", a.Updateable, b.Updateable))
	}
	if a.Createable != b.Createable {
		diffs = append(diffs, fmt.Sprintf("createable: %v → %v", a.Createable, b.Createable))
	}
	if a.Custom != b.Custom {
		diffs = append(diffs, fmt.Sprintf("custom: %v → %v", a.Custom, b.Custom))
	}
	if a.CalculatedFormula != b.CalculatedFormula {
		diffs = append(diffs, "formula changed")
	}
	if !sameRefs(a.ReferenceTo, b.ReferenceTo) {
		diffs = append(diffs, fmt.Sprintf("references: %v → %v", a.ReferenceTo, b.ReferenceTo))
	}
	if !samePicklist(a.PicklistValues, b.PicklistValues) {
		diffs = append(diffs, "picklist values differ")
	}
	return diffs
}

func sameRefs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

func samePicklist(a, b []sf.PicklistValue) bool {
	if len(a) != len(b) {
		return false
	}
	mk := func(p []sf.PicklistValue) []string {
		out := make([]string, 0, len(p))
		for _, v := range p {
			if v.Active {
				out = append(out, v.Value)
			}
		}
		sort.Strings(out)
		return out
	}
	x := mk(a)
	y := mk(b)
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

func (c Compare) UpdateList(msg tea.Msg) (Compare, tea.Cmd) {
	var cmd tea.Cmd
	c.List, cmd = c.List.Update(msg)
	return c, cmd
}

func (c Compare) UpdateViewport(msg tea.Msg) (Compare, tea.Cmd) {
	if vpJumpKeys(&c.Viewport, msg) {
		return c, nil
	}
	var cmd tea.Cmd
	c.Viewport, cmd = c.Viewport.Update(msg)
	return c, cmd
}

func (c Compare) View(listFocus, detailFocus bool) string {
	left := BorderFor(listFocus).Render(c.List.View())
	right := BorderFor(detailFocus).Render(c.Viewport.View())
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

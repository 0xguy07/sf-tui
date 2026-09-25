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

type saveItem struct {
	s sf.Save
}

func (i saveItem) Title() string {
	return i.s.At.Local().Format("2006-01-02 15:04:05")
}

func (i saveItem) Description() string {
	who := i.s.CreatedByID
	if u := i.s.User; u != nil {
		who = u.Name
		if u.Badge != "" {
			who = "[" + u.Badge + "] " + who
		}
	}
	n := len(i.s.Changes)
	for _, c := range i.s.Changes {
		if c.Created {
			return fmt.Sprintf("%s · created +%d", who, n-1)
		}
	}
	return fmt.Sprintf("%s · %d field(s)", who, n)
}

func (i saveItem) FilterValue() string {
	parts := []string{i.Title(), i.Description()}
	for _, c := range i.s.Changes {
		parts = append(parts, c.Field)
	}
	return strings.Join(parts, " ")
}

type autoRow struct {
	text string
	item *sf.AutomationItem
}

type Why struct {
	Input  textinput.Model
	Saves  list.Model
	Detail viewport.Model

	autoFilter    textinput.Model
	autoFiltering bool
	autoCursor    int
	autoOffset    int
	ShowInactive  bool

	trace             sf.Trace
	hasTrace          bool
	seq               int
	resolving         bool
	loadingHistory    bool
	loadingAutomation bool
	message           string

	width, height int
	listW, bodyH  int
}

func NewWhy(width, height int) Why {
	in := textinput.New()
	in.Placeholder = "15- or 18-char record Id…"
	in.CharLimit = 18
	in.Width = 24
	in.Prompt = "Record Id: "

	l := list.New(nil, list.NewDefaultDelegate(), width/3, height)
	l.Title = "Saves"
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)

	af := textinput.New()
	af.Prompt = "/"
	af.Placeholder = "filter automation…"

	w := Why{Input: in, Saves: l, Detail: viewport.New(width/2, height), autoFilter: af}
	w.SetSize(width, height)
	return w
}

func (w *Why) SetSize(width, height int) {
	w.width, w.height = width, height
	w.bodyH = height - 5
	if w.bodyH < 3 {
		w.bodyH = 3
	}
	w.listW = width / 3
	if w.listW < 30 {
		w.listW = 30
	}
	detailW := width - w.listW - 4
	if detailW < 20 {
		detailW = 20
	}
	w.Saves.SetSize(w.listW, w.bodyH)
	w.Detail.Width = detailW
	w.Detail.Height = w.bodyH
	w.renderDetail()
}

func (w *Why) FocusInput() tea.Cmd { return w.Input.Focus() }
func (w *Why) BlurInput()          { w.Input.Blur() }
func (w Why) Value() string        { return strings.TrimSpace(w.Input.Value()) }
func (w *Why) SetID(id string)     { w.Input.SetValue(id) }
func (w Why) Seq() int             { return w.seq }

// LoadedID is the Id of the record currently shown, or "".
func (w Why) LoadedID() string {
	if !w.hasTrace {
		return ""
	}
	return w.trace.Record.ID
}

func (w Why) Trace() sf.Trace { return w.trace }

// Filtering reports whether a filter input in the pane is capturing keys.
func (w Why) Filtering() bool {
	return w.autoFiltering || w.Saves.FilterState() == list.Filtering
}

func (w Why) Busy() bool { return w.resolving || w.loadingHistory || w.loadingAutomation }

// Begin starts a new run and returns its sequence number.
func (w *Why) Begin() int {
	w.seq++
	w.resolving = true
	w.loadingHistory = false
	w.loadingAutomation = false
	w.message = ""
	return w.seq
}

func (w *Why) SetResolved(msg sf.WhyResolvedMsg) {
	w.resolving = false
	if msg.Err != nil {
		w.hasTrace = false
		w.message = msg.Err.Error()
		w.Saves.SetItems(nil)
		w.renderDetail()
		return
	}
	w.trace = msg.Trace
	w.hasTrace = true
	w.loadingHistory = true
	w.loadingAutomation = true
	w.autoCursor, w.autoOffset = 0, 0
	w.Saves.SetItems(nil)
	w.renderDetail()
}

func (w *Why) SetHistory(p sf.HistoryPart) {
	w.loadingHistory = false
	w.trace.ApplyHistory(p)
	w.afterPart()
}

func (w *Why) SetAutomation(p sf.AutomationPart) {
	w.loadingAutomation = false
	w.trace.ApplyAutomation(p)
	w.afterPart()
}

func (w *Why) afterPart() {
	if !w.loadingHistory && !w.loadingAutomation {
		w.trace.Attribute()
	}
	items := make([]list.Item, 0, len(w.trace.Saves))
	for _, s := range w.trace.Saves {
		items = append(items, saveItem{s: s})
	}
	idx := w.Saves.Index()
	w.Saves.SetItems(items)
	if idx < len(items) {
		w.Saves.Select(idx)
	}
	w.renderDetail()
}

func (w *Why) ToggleInactive() {
	w.ShowInactive = !w.ShowInactive
	w.autoCursor, w.autoOffset = 0, 0
}

func (w Why) selectedSave() *sf.Save {
	it, ok := w.Saves.SelectedItem().(saveItem)
	if !ok {
		return nil
	}
	return &it.s
}

var (
	whyTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	whyErr    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	whyBadge  = lipgloss.NewStyle().Foreground(lipgloss.Color("16")).Background(lipgloss.Color("220")).Padding(0, 1).Bold(true)
	whyArrow  = lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	whyCursor = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
)

func (w *Why) renderDetail() {
	var b strings.Builder
	switch {
	case w.resolving:
		b.WriteString(permDim.Render("resolving record…"))
	case !w.hasTrace && w.message != "":
		b.WriteString(whyErr.Render(w.message))
	case !w.hasTrace:
		b.WriteString(permDim.Render("Enter a record Id and press ctrl+r."))
	case w.loadingHistory:
		b.WriteString(permDim.Render("loading field history…"))
	default:
		if e := w.trace.ErrorFor("history"); e != "" {
			b.WriteString(whyErr.Render(e) + "\n")
		}
		if e := w.trace.ErrorFor("users"); e != "" {
			b.WriteString(whyErr.Render("users: "+e) + "\n")
		}
		s := w.selectedSave()
		if s == nil {
			if w.trace.ErrorFor("history") == "" {
				b.WriteString(permDim.Render("No tracked field changes on this record."))
			}
			break
		}
		renderSave(&b, *s, w.loadingAutomation, w.trace.ErrorFor(sf.SecBeforeFlows) != "" || w.trace.ErrorFor(sf.SecWorkflow) != "")
	}
	w.Detail.SetContent(lipgloss.NewStyle().Width(w.Detail.Width).Render(b.String()))
}

func renderSave(b *strings.Builder, s sf.Save, autoLoading, autoPartial bool) {
	fmt.Fprintln(b, whyTitle.Render(s.At.Local().Format("Mon 2006-01-02 15:04:05 MST")))
	if u := s.User; u != nil {
		line := u.Name
		if u.Badge != "" {
			line += " " + whyBadge.Render(u.Badge)
		}
		fmt.Fprintln(b, line)
		meta := []string{u.Username, "UserType: " + u.UserType}
		if u.Profile != "" {
			meta = append(meta, "Profile: "+u.Profile)
		}
		if u.License != "" {
			meta = append(meta, "License: "+u.License)
		}
		if !u.IsActive {
			meta = append(meta, "inactive")
		}
		fmt.Fprintln(b, permDim.Render(strings.Join(meta, " · ")))
	} else {
		fmt.Fprintln(b, s.CreatedByID)
	}
	if s.Hint != "" {
		fmt.Fprintln(b, permDim.Render("hint: "+s.Hint))
	}
	fmt.Fprintln(b)
	for _, c := range s.Changes {
		if c.Created {
			fmt.Fprintln(b, permSection.Render("Record created"))
			continue
		}
		fmt.Fprintf(b, "%s: %s %s %s\n", permSection.Render(c.Field), fmtValue(c.Old), whyArrow.Render("→"), fmtValue(c.New))
		if c.OldID != "" || c.NewID != "" {
			fmt.Fprintln(b, permDim.Render("  ids: "+orDash(c.OldID)+" → "+orDash(c.NewID)))
		}
		switch {
		case autoLoading:
			fmt.Fprintln(b, permDim.Render("  loading automation…"))
		case len(c.Writers) == 0:
			msg := "  no automation on this object writes this field"
			if autoPartial {
				msg += " (automation partially unavailable)"
			}
			fmt.Fprintln(b, permDim.Render(msg))
		default:
			for _, wr := range c.Writers {
				line := "  ↳ " + wr.Name + permDim.Render(" · "+wr.Section)
				if wr.Note != "" {
					line += permDim.Render(" — " + wr.Note)
				}
				fmt.Fprintln(b, line)
			}
		}
	}
}

func fmtValue(v any) string {
	if v == nil {
		return permDim.Render("∅")
	}
	s := strings.ReplaceAll(fmt.Sprintf("%v", v), "\n", " ")
	if f, ok := v.(float64); ok && f == float64(int64(f)) {
		s = fmt.Sprintf("%d", int64(f))
	}
	return truncate(s, 80)
}

func orDash(s string) string {
	if s == "" {
		return "∅"
	}
	return s
}

// --- automation pane ---

func (w Why) autoRows() []autoRow {
	var rows []autoRow
	if !w.hasTrace {
		return nil
	}
	if w.loadingAutomation {
		return []autoRow{{text: permDim.Render("loading automation…")}}
	}
	filter := strings.ToLower(strings.TrimSpace(w.autoFilter.Value()))
	for si := range w.trace.Automation {
		sec := &w.trace.Automation[si]
		title := permSection.Render(fmt.Sprintf("%d. %s", si+1, sec.Title))
		if sec.Note != "" {
			title += permDim.Render("  — " + sec.Note)
		}
		rows = append(rows, autoRow{text: title})
		if e := w.trace.ErrorFor(sec.Key); e != "" {
			rows = append(rows, autoRow{text: "   " + whyErr.Render(e)})
		}
		shown := 0
		for ii := range sec.Items {
			it := &sec.Items[ii]
			if !it.Active && !w.ShowInactive {
				continue
			}
			if filter != "" && !strings.Contains(strings.ToLower(it.Display()+" "+it.Name), filter) {
				continue
			}
			rows = append(rows, autoRow{text: autoLine(*it), item: it})
			shown++
		}
		if shown == 0 {
			empty := "none"
			if sec.Key == sf.SecAssignment && w.trace.Object.Name != "Lead" && w.trace.Object.Name != "Case" {
				empty = "Lead and Case only"
			}
			rows = append(rows, autoRow{text: "   " + permDim.Render(empty)})
		}
	}
	rows = append(rows, autoRow{text: ""}, autoRow{text: permDim.Render(sf.TriggerOrderFooter)})
	return rows
}

func autoLine(it sf.AutomationItem) string {
	var ev strings.Builder
	hasEvents := it.Kind != sf.KindValidation && it.Kind != sf.KindAssignment
	for _, e := range []string{"C", "U", "D"} {
		if !hasEvents {
			break
		}
		on := false
		for _, x := range it.Events {
			on = on || x == e
		}
		if on {
			ev.WriteString("[" + e + "]")
		} else {
			ev.WriteString(permDim.Render("[ ]"))
		}
	}
	line := it.Display()
	if hasEvents {
		line = ev.String() + " " + line
	}
	var extra []string
	if it.TriggerOrder != nil {
		extra = append(extra, fmt.Sprintf("order %d", *it.TriggerOrder))
	}
	if !it.Active {
		extra = append(extra, "inactive")
	}
	if len(it.Writes) > 0 {
		fs := make([]string, 0, len(it.Writes))
		for _, fw := range it.Writes {
			fs = append(fs, fw.Field)
		}
		extra = append(extra, "writes "+strings.Join(fs, ", "))
	}
	if it.UpdatesRecordVariable {
		extra = append(extra, "updates a record variable")
	}
	if it.Detail != "" {
		extra = append(extra, it.Detail)
	}
	if len(extra) > 0 {
		line += permDim.Render("  · " + strings.Join(extra, " · "))
	}
	return line
}

func selectableIdx(rows []autoRow) []int {
	var idx []int
	for i, r := range rows {
		if r.item != nil {
			idx = append(idx, i)
		}
	}
	return idx
}

// SelectedAutomation returns the highlighted automation item, or nil.
func (w Why) SelectedAutomation() *sf.AutomationItem {
	rows := w.autoRows()
	sel := selectableIdx(rows)
	if len(sel) == 0 {
		return nil
	}
	c := w.autoCursor
	if c >= len(sel) {
		c = len(sel) - 1
	}
	return rows[sel[c]].item
}

func (w Why) UpdateInput(msg tea.Msg) (Why, tea.Cmd) {
	var cmd tea.Cmd
	w.Input, cmd = w.Input.Update(msg)
	return w, cmd
}

func (w Why) UpdateTimeline(msg tea.Msg) (Why, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok && w.Saves.FilterState() != list.Filtering {
		switch km.String() {
		case "pgup", "pgdown", "ctrl+u", "ctrl+d":
			var cmd tea.Cmd
			w.Detail, cmd = w.Detail.Update(msg)
			return w, cmd
		}
	}
	prev := w.Saves.Index()
	var cmd tea.Cmd
	w.Saves, cmd = w.Saves.Update(msg)
	if w.Saves.Index() != prev {
		w.renderDetail()
		w.Detail.GotoTop()
	}
	return w, cmd
}

func (w Why) UpdateAutomation(msg tea.Msg) (Why, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return w, nil
	}
	if w.autoFiltering {
		switch km.String() {
		case "esc":
			w.autoFiltering = false
			w.autoFilter.Blur()
			w.autoFilter.SetValue("")
			w.autoCursor, w.autoOffset = 0, 0
			return w, nil
		case "enter":
			w.autoFiltering = false
			w.autoFilter.Blur()
			return w, nil
		}
		var cmd tea.Cmd
		w.autoFilter, cmd = w.autoFilter.Update(msg)
		w.autoCursor, w.autoOffset = 0, 0
		return w, cmd
	}
	n := len(selectableIdx(w.autoRows()))
	switch km.String() {
	case "/":
		w.autoFiltering = true
		return w, w.autoFilter.Focus()
	case "esc":
		w.autoFilter.SetValue("")
	case "up", "k":
		if w.autoCursor > 0 {
			w.autoCursor--
		}
	case "down", "j":
		if w.autoCursor < n-1 {
			w.autoCursor++
		}
	case "g", "home":
		w.autoCursor = 0
	case "G", "end":
		if n > 0 {
			w.autoCursor = n - 1
		}
	}
	return w, nil
}

func (w Why) header() string {
	lines := []string{w.Input.View()}
	switch {
	case w.hasTrace:
		t := w.trace
		obj := whyTitle.Render(t.Object.Label) + permDim.Render(" ("+t.Object.Name+") · "+t.Record.ID)
		if t.HistoryTruncated {
			obj += permDim.Render(" · showing latest 200")
		}
		lines = append(lines, obj)
		tracked := fmt.Sprintf("Tracked (%d): %s", len(t.TrackedFields), strings.Join(t.TrackedFields, ", "))
		if e := t.ErrorFor("tracked"); e != "" {
			tracked = whyErr.Render("Tracked fields: " + e)
		} else if w.loadingHistory {
			tracked = "Tracked: loading…"
		}
		lines = append(lines, permDim.Render(truncate(tracked, w.width-2)))
	case w.message != "":
		lines = append(lines, whyErr.Render(w.message), "")
	default:
		lines = append(lines, "", "")
	}
	return strings.Join(lines, "\n")
}

func (w Why) automationView() string {
	rows := w.autoRows()
	sel := selectableIdx(rows)
	h := w.bodyH
	if w.autoFiltering || w.autoFilter.Value() != "" {
		h--
	}
	cursorRow := -1
	if len(sel) > 0 {
		c := w.autoCursor
		if c >= len(sel) {
			c = len(sel) - 1
		}
		cursorRow = sel[c]
	}
	off := w.autoOffset
	if cursorRow >= 0 {
		if cursorRow < off {
			off = cursorRow
		}
		if cursorRow >= off+h {
			off = cursorRow - h + 1
		}
	}
	var b strings.Builder
	if w.autoFiltering || w.autoFilter.Value() != "" {
		b.WriteString(w.autoFilter.View() + "\n")
	}
	for i := off; i < len(rows) && i < off+h; i++ {
		line := "  " + rows[i].text
		if i == cursorRow {
			line = whyCursor.Render("▸ ") + rows[i].text
		}
		b.WriteString(line + "\n")
	}
	if !w.hasTrace {
		b.WriteString(permDim.Render("Run a trace to list automation on the object."))
	}
	return lipgloss.NewStyle().Width(w.width - 2).Height(w.bodyH).MaxHeight(w.bodyH).Render(strings.TrimRight(b.String(), "\n"))
}

// View renders the tab. focus: 0 input, 1 timeline, 2 automation.
func (w Why) View(focus int) string {
	head := BorderFor(focus == 0).Width(w.width - 2).Render(w.header())
	var body string
	if focus == 2 {
		body = BorderFor(true).Render(w.automationView())
	} else {
		left := BorderFor(focus == 1).Render(w.Saves.View())
		right := BorderFor(false).Render(w.Detail.View())
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}
	return lipgloss.JoinVertical(lipgloss.Left, head, body)
}

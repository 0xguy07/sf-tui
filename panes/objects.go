package panes

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/0xguy07/sf-tui/sf"
)

const typeaheadReset = 750 * time.Millisecond

type sobjectItem struct {
	s sf.SObjectSummary
}

func (i sobjectItem) Title() string {
	if i.s.Label != "" && i.s.Label != i.s.Name {
		return i.s.Name
	}
	return i.s.Name
}

func (i sobjectItem) Description() string {
	tag := "std"
	if i.s.Custom {
		tag = "custom"
	}
	if i.s.Label != "" && i.s.Label != i.s.Name {
		return fmt.Sprintf("%s · %s", tag, i.s.Label)
	}
	return tag
}

func (i sobjectItem) FilterValue() string { return i.s.Name + " " + i.s.Label }

type Objects struct {
	List      list.Model
	Detail    viewport.Model
	Describe  *sf.Describe
	LastName  string
	listFocus bool

	typed   string
	typedAt time.Time
}

func NewObjects(width, height int) Objects {
	l := list.New(nil, list.NewDefaultDelegate(), width/3, height)
	l.Title = "SObjects"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	v := viewport.New(width-width/3-2, height)
	return Objects{List: l, Detail: v, listFocus: true}
}

func (o *Objects) SetSize(width, height int) {
	listW := width / 3
	if listW < 20 {
		listW = 20
	}
	o.List.SetSize(listW, height)
	o.Detail.Width = width - listW - 2
	o.Detail.Height = height
	o.renderDetail()
}

func (o *Objects) SetSObjects(ss []sf.SObjectSummary) {
	items := make([]list.Item, 0, len(ss))
	for _, s := range ss {
		items = append(items, sobjectItem{s: s})
	}
	o.List.SetItems(items)
}

func (o *Objects) SelectedName() string {
	it, ok := o.List.SelectedItem().(sobjectItem)
	if !ok {
		return ""
	}
	return it.s.Name
}

func (o *Objects) SetDescribe(d sf.Describe) {
	o.Describe = &d
	o.LastName = d.Name
	o.renderDetail()
}

func (o *Objects) renderDetail() {
	if o.Describe == nil {
		o.Detail.SetContent(lipgloss.NewStyle().Faint(true).Render(
			"Select an SObject and press enter to load fields."))
		return
	}
	var b strings.Builder
	header := lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("%s  (%s)", o.Describe.Label, o.Describe.Name))
	b.WriteString(header + "\n\n")

	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	typeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	tagStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	helpStyle := lipgloss.NewStyle().Faint(true)

	for _, f := range o.Describe.Fields {
		flags := []string{}
		if !f.Nillable {
			flags = append(flags, "required")
		}
		if f.Custom {
			flags = append(flags, "custom")
		}
		if f.CalculatedFormula != "" {
			flags = append(flags, "formula")
		}
		if !f.Updateable {
			flags = append(flags, "read-only")
		}
		flagStr := ""
		if len(flags) > 0 {
			flagStr = "  " + tagStyle.Render("["+strings.Join(flags, ",")+"]")
		}

		typeLabel := f.Type
		if f.Type == "reference" && len(f.ReferenceTo) > 0 {
			typeLabel = "reference→" + strings.Join(f.ReferenceTo, ",")
		}
		if f.Length > 0 && (f.Type == "string" || f.Type == "textarea") {
			typeLabel = fmt.Sprintf("%s(%d)", f.Type, f.Length)
		}

		b.WriteString(fmt.Sprintf("%s  %s%s\n",
			nameStyle.Render(f.Name),
			typeStyle.Render(typeLabel),
			flagStr))
		if f.Label != "" && f.Label != f.Name {
			b.WriteString(helpStyle.Render("  label: "+f.Label) + "\n")
		}
		if f.InlineHelpText != "" {
			b.WriteString(helpStyle.Render("  help:  "+f.InlineHelpText) + "\n")
		}
		if len(f.PicklistValues) > 0 {
			vs := make([]string, 0, len(f.PicklistValues))
			for _, p := range f.PicklistValues {
				if p.Active {
					vs = append(vs, p.Value)
				}
			}
			preview := strings.Join(vs, ", ")
			if len(preview) > 120 {
				preview = preview[:117] + "…"
			}
			b.WriteString(helpStyle.Render("  values: "+preview) + "\n")
		}
		b.WriteString("\n")
	}
	o.Detail.SetContent(b.String())
	o.Detail.GotoTop()
}

func (o *Objects) FocusList()   { o.listFocus = true }
func (o *Objects) FocusDetail() { o.listFocus = false }
func (o Objects) ListFocused() bool { return o.listFocus }

func (o Objects) Update(msg tea.Msg) (Objects, tea.Cmd) {
	var cmd tea.Cmd
	if o.listFocus {
		// Typeahead jump: only when not in the list's filter mode.
		if km, ok := msg.(tea.KeyMsg); ok && o.List.FilterState() != list.Filtering {
			switch km.Type {
			case tea.KeyRunes:
				if len(km.Runes) == 1 {
					r := km.Runes[0]
					if isJumpRune(r) {
						o = o.typeaheadJump(r)
						return o, nil
					}
				}
			case tea.KeyLeft:
				o.jumpLetter(-1)
				return o, nil
			case tea.KeyRight:
				o.jumpLetter(+1)
				return o, nil
			}
		}
		o.List, cmd = o.List.Update(msg)
	} else {
		if vpJumpKeys(&o.Detail, msg) {
			return o, nil
		}
		o.Detail, cmd = o.Detail.Update(msg)
	}
	return o, cmd
}

func isJumpRune(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '_'
}

// typeaheadJump accumulates typed runes within a timeout window and selects the
// first item whose name starts with the accumulated prefix (case-insensitive).
// If nothing matches, falls back to the single newest rune.
func (o Objects) typeaheadJump(r rune) Objects {
	now := time.Now()
	if now.Sub(o.typedAt) > typeaheadReset {
		o.typed = ""
	}
	o.typed += strings.ToLower(string(r))
	o.typedAt = now

	if idx := findPrefix(o.List.Items(), o.typed); idx >= 0 {
		o.List.Select(idx)
		return o
	}
	// Fallback: restart with just this rune.
	o.typed = strings.ToLower(string(r))
	if idx := findPrefix(o.List.Items(), o.typed); idx >= 0 {
		o.List.Select(idx)
	}
	return o
}

// jumpLetter moves the cursor to the next/previous item starting with a
// different first letter than the currently selected one (A → B → C…).
func (o *Objects) jumpLetter(dir int) {
	items := o.List.Items()
	if len(items) == 0 {
		return
	}
	cur := o.List.Index()
	curFirst := firstLetter(items[cur])
	i := cur
	for step := 0; step < len(items); step++ {
		i += dir
		if i < 0 {
			i = len(items) - 1
		} else if i >= len(items) {
			i = 0
		}
		if firstLetter(items[i]) != curFirst {
			o.List.Select(i)
			return
		}
	}
}

func firstLetter(it list.Item) byte {
	if si, ok := it.(sobjectItem); ok && si.s.Name != "" {
		c := si.s.Name[0]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		return c
	}
	return 0
}

func findPrefix(items []list.Item, prefix string) int {
	for i, it := range items {
		si, ok := it.(sobjectItem)
		if !ok {
			continue
		}
		if strings.HasPrefix(strings.ToLower(si.s.Name), prefix) {
			return i
		}
	}
	return -1
}

func (o Objects) View(listActive, detailActive bool) string {
	listStyle := idleBorder
	detailStyle := idleBorder
	if listActive {
		listStyle = activeBorder
	}
	if detailActive {
		detailStyle = activeBorder
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		listStyle.Render(o.List.View()),
		detailStyle.Render(o.Detail.View()),
	)
}

package panes

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/0xguy07/sf-tui/sf"
)

type queryItem struct {
	q sf.SavedQuery
}

func (i queryItem) Title() string {
	if i.q.Saved {
		return "★ " + i.q.Name
	}
	preview := strings.ReplaceAll(i.q.SOQL, "\n", " ")
	if len(preview) > 70 {
		preview = preview[:67] + "…"
	}
	return preview
}

func (i queryItem) Description() string {
	if i.q.Saved {
		return fmt.Sprintf("%s · %s", i.q.Org, oneLine(i.q.SOQL, 60))
	}
	return fmt.Sprintf("%s · %s", i.q.Org, i.q.RanAt.Format("Jan 2 15:04"))
}

func (i queryItem) FilterValue() string { return i.q.Name + " " + i.q.SOQL + " " + i.q.Org }

func oneLine(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > max {
		s = s[:max-1] + "…"
	}
	return s
}

type Picker struct {
	List    list.Model
	Visible bool
}

func NewPicker(width, height int) Picker {
	l := list.New(nil, list.NewDefaultDelegate(), width, height)
	l.Title = "Queries (saved + history)"
	l.SetShowHelp(true)
	l.SetFilteringEnabled(true)
	return Picker{List: l}
}

func (p *Picker) SetEntries(entries []sf.SavedQuery) {
	items := make([]list.Item, 0, len(entries))
	for _, e := range entries {
		items = append(items, queryItem{q: e})
	}
	p.List.SetItems(items)
}

func (p *Picker) SetSize(width, height int) {
	p.List.SetSize(width, height)
}

func (p *Picker) Selected() *sf.SavedQuery {
	it, ok := p.List.SelectedItem().(queryItem)
	if !ok {
		return nil
	}
	return &it.q
}

func (p Picker) Update(msg tea.Msg) (Picker, tea.Cmd) {
	var cmd tea.Cmd
	p.List, cmd = p.List.Update(msg)
	return p, cmd
}

func (p Picker) View() string {
	return activeBorder.Render(p.List.View())
}

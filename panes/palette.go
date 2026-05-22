package panes

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// Command represents a single entry in the command palette.
// ID is opaque — main.go switches on it to dispatch the action.
type Command struct {
	ID    string
	Label string
	Hint  string // shortcut hint shown on the right (e.g. "ctrl+r")
	Help  string // longer description shown under the label
}

type commandItem struct {
	c Command
}

func (c commandItem) Title() string {
	if c.c.Hint != "" {
		return c.c.Label + "   [" + c.c.Hint + "]"
	}
	return c.c.Label
}
func (c commandItem) Description() string  { return c.c.Help }
func (c commandItem) FilterValue() string  { return c.c.Label + " " + c.c.Help + " " + c.c.ID }

type Palette struct {
	List    list.Model
	Visible bool
}

func NewPalette(width, height int) Palette {
	l := list.New(nil, list.NewDefaultDelegate(), width, height)
	l.Title = "Command Palette"
	l.SetShowHelp(true)
	l.SetFilteringEnabled(true)
	return Palette{List: l}
}

func (p *Palette) SetCommands(cmds []Command) {
	items := make([]list.Item, 0, len(cmds))
	for _, c := range cmds {
		items = append(items, commandItem{c: c})
	}
	p.List.SetItems(items)
	p.List.ResetFilter()
	p.List.Select(0)
}

func (p *Palette) SetSize(width, height int) {
	p.List.SetSize(width, height)
}

func (p *Palette) Selected() *Command {
	it, ok := p.List.SelectedItem().(commandItem)
	if !ok {
		return nil
	}
	return &it.c
}

func (p Palette) Update(msg tea.Msg) (Palette, tea.Cmd) {
	var cmd tea.Cmd
	p.List, cmd = p.List.Update(msg)
	return p, cmd
}

func (p Palette) View() string {
	return activeBorder.Render(p.List.View())
}

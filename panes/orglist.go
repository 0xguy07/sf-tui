package panes

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/0xguy07/sf-tui/sf"
)

type orgItem struct {
	org sf.Org
}

func (o orgItem) Title() string {
	if o.org.Alias != "" {
		return o.org.Alias
	}
	return o.org.Username
}

func (o orgItem) Description() string { return o.org.Username }
func (o orgItem) FilterValue() string { return o.Title() + " " + o.org.Username }

type OrgList struct {
	List list.Model
}

func NewOrgList(width, height int) OrgList {
	l := list.New(nil, list.NewDefaultDelegate(), width, height)
	l.Title = "Orgs"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	return OrgList{List: l}
}

func (o *OrgList) SetOrgs(orgs []sf.Org) {
	items := make([]list.Item, 0, len(orgs))
	for _, org := range orgs {
		items = append(items, orgItem{org: org})
	}
	o.List.SetItems(items)
}

func (o *OrgList) Selected() *sf.Org {
	it, ok := o.List.SelectedItem().(orgItem)
	if !ok {
		return nil
	}
	return &it.org
}

func (o OrgList) Update(msg tea.Msg) (OrgList, tea.Cmd) {
	var cmd tea.Cmd
	o.List, cmd = o.List.Update(msg)
	return o, cmd
}

func (o OrgList) View() string { return o.List.View() }

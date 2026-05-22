package panes

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/0xguy07/sf-tui/sf"
)

type userItem struct {
	u sf.UserBrief
}

func (i userItem) Title() string { return i.u.Name }
func (i userItem) Description() string {
	if i.u.ProfileName != "" {
		return i.u.Username + " · " + i.u.ProfileName
	}
	return i.u.Username
}
func (i userItem) FilterValue() string { return i.u.Name + " " + i.u.Username + " " + i.u.ProfileName }

type Permissions struct {
	List     list.Model
	Viewport viewport.Model
	width    int
	height   int
	users    []sf.UserBrief
	loaded   bool

	lastDetail   sf.PermissionsLoadedMsg // last object-perm load, replayed when FLS arrives
	hasDetail    bool
	flsObject    string
	fls          []sf.FieldPerm
	hasFLS       bool
}

func NewPermissions(width, height int) Permissions {
	l := list.New(nil, list.NewDefaultDelegate(), width/2, height)
	l.Title = "Users"
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)
	v := viewport.New(width/2, height)
	v.SetContent(lipgloss.NewStyle().Faint(true).Render("Pick a user, press enter to load effective permissions."))
	return Permissions{List: l, Viewport: v, width: width, height: height}
}

func (p *Permissions) SetSize(width, height int) {
	p.width = width
	p.height = height
	listW := width / 3
	if listW < 30 {
		listW = 30
	}
	detailW := width - listW - 4
	if detailW < 20 {
		detailW = 20
	}
	p.List.SetSize(listW, height)
	p.Viewport.Width = detailW
	p.Viewport.Height = height
}

func (p *Permissions) SetUsers(users []sf.UserBrief) {
	p.users = users
	items := make([]list.Item, 0, len(users))
	for _, u := range users {
		items = append(items, userItem{u: u})
	}
	p.List.SetItems(items)
	p.loaded = true
}

func (p Permissions) Loaded() bool { return p.loaded }

func (p *Permissions) Selected() *sf.UserBrief {
	it, ok := p.List.SelectedItem().(userItem)
	if !ok {
		return nil
	}
	return &it.u
}

func (p *Permissions) SetDetail(msg sf.PermissionsLoadedMsg) {
	p.lastDetail = msg
	p.hasDetail = true
	// New user / re-load — clear any previous FLS view.
	p.hasFLS = false
	p.flsObject = ""
	p.fls = nil
	p.Viewport.SetContent(renderPermDetail(msg, "", nil))
	p.Viewport.GotoTop()
}

// SetFieldPerms appends a field-level permissions section to the existing
// object-perms view. Caller must have called SetDetail first.
func (p *Permissions) SetFieldPerms(msg sf.FieldPermissionsLoadedMsg) {
	p.flsObject = msg.SObject
	p.fls = msg.Fields
	p.hasFLS = true
	if p.hasDetail {
		p.Viewport.SetContent(renderPermDetail(p.lastDetail, p.flsObject, p.fls))
	}
}

// SelectedDetailUser returns the user the current detail view describes, or
// nil if no detail has been loaded yet. Used by callers to fetch FLS for the
// already-loaded user without re-resolving from the list.
func (p *Permissions) SelectedDetailUser() *sf.UserBrief {
	if !p.hasDetail {
		return nil
	}
	u := p.lastDetail.User
	return &u
}

func (p *Permissions) SetStatus(s string) {
	p.Viewport.SetContent(lipgloss.NewStyle().Faint(true).Render(s))
}

var (
	permHeader  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	permSection = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
	permDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	permGreen   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	permRed     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

func renderPermDetail(msg sf.PermissionsLoadedMsg, flsObject string, fls []sf.FieldPerm) string {
	var b strings.Builder
	fmt.Fprintln(&b, permHeader.Render(msg.User.Name))
	fmt.Fprintln(&b, permDim.Render(msg.User.Username+" · "+msg.User.ProfileName))
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, permSection.Render(fmt.Sprintf("Permission set assignments (%d)", len(msg.Assignments))))
	for _, a := range msg.Assignments {
		label := a.PermSetLabel
		if a.IsProfile {
			label = "Profile: " + a.ProfileName
		}
		fmt.Fprintln(&b, "  • "+label)
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, permSection.Render(fmt.Sprintf("Object permissions (%d objects)", len(msg.Objects))))
	fmt.Fprintln(&b, permDim.Render("       R  C  E  D  V  M   sources"))
	for _, op := range msg.Objects {
		row := fmt.Sprintf("  %-30s %s %s %s %s %s %s   %s",
			truncate(op.SObject, 30),
			flag(op.Read),
			flag(op.Create),
			flag(op.Edit),
			flag(op.Delete),
			flag(op.ViewAll),
			flag(op.ModifyAll),
			permDim.Render(strings.Join(op.Sources, ", ")),
		)
		fmt.Fprintln(&b, row)
	}

	if flsObject != "" {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, permSection.Render(fmt.Sprintf("Field permissions on %s (%d)", flsObject, len(fls))))
		if len(fls) == 0 {
			fmt.Fprintln(&b, permDim.Render("  no FieldPermissions records — fields fall back to profile defaults"))
		} else {
			fmt.Fprintln(&b, permDim.Render("       R  E   sources"))
			for _, f := range fls {
				row := fmt.Sprintf("  %-30s %s %s   %s",
					truncate(f.Field, 30),
					flag(f.Read),
					flag(f.Edit),
					permDim.Render(strings.Join(f.Sources, ", ")),
				)
				fmt.Fprintln(&b, row)
			}
		}
	}

	return b.String()
}

func flag(on bool) string {
	if on {
		return permGreen.Render("●")
	}
	return permRed.Render("·")
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func (p Permissions) UpdateList(msg tea.Msg) (Permissions, tea.Cmd) {
	var cmd tea.Cmd
	p.List, cmd = p.List.Update(msg)
	return p, cmd
}

func (p Permissions) UpdateViewport(msg tea.Msg) (Permissions, tea.Cmd) {
	if vpJumpKeys(&p.Viewport, msg) {
		return p, nil
	}
	var cmd tea.Cmd
	p.Viewport, cmd = p.Viewport.Update(msg)
	return p, cmd
}

func (p Permissions) View(listFocus, detailFocus bool) string {
	left := BorderFor(listFocus).Render(p.List.View())
	right := BorderFor(detailFocus).Render(p.Viewport.View())
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

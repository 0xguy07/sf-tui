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

type metaItem struct {
	it sf.MetadataItem
}

func (m metaItem) Title() string {
	prefix := actionGlyph(m.it.Type)
	name := m.it.FullName
	if m.it.MetaType != "" {
		name = m.it.MetaType + ": " + name
	}
	return prefix + " " + name
}
func (m metaItem) Description() string { return m.it.Path }
func (m metaItem) FilterValue() string { return m.it.FullName + " " + m.it.MetaType + " " + m.it.Path }

type Metadata struct {
	List     list.Model
	Viewport viewport.Model
	width    int
	height   int

	projectDir string
	items      []sf.MetadataItem
	loaded     bool
}

func NewMetadata(width, height int) Metadata {
	l := list.New(nil, list.NewDefaultDelegate(), width/2, height)
	l.Title = "Deploy plan"
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)
	v := viewport.New(width/2, height)
	v.SetContent(lipgloss.NewStyle().Faint(true).Render(
		"Press ctrl+r to load a deploy preview.\n" +
			"Project dir defaults to your current working directory."))
	return Metadata{List: l, Viewport: v, width: width, height: height}
}

func (m *Metadata) SetSize(width, height int) {
	m.width = width
	m.height = height
	listW := width / 2
	if listW < 30 {
		listW = 30
	}
	detailW := width - listW - 4
	if detailW < 20 {
		detailW = 20
	}
	m.List.SetSize(listW, height)
	m.Viewport.Width = detailW
	m.Viewport.Height = height
}

func (m *Metadata) ProjectDir() string { return m.projectDir }

func (m *Metadata) SetPreview(msg sf.DeployPreviewLoadedMsg) {
	m.projectDir = msg.ProjectDir
	m.items = msg.Items
	m.loaded = true
	items := make([]list.Item, 0, len(msg.Items))
	for _, it := range msg.Items {
		items = append(items, metaItem{it: it})
	}
	m.List.SetItems(items)
	m.Viewport.SetContent(renderMetaSummary(msg))
	m.Viewport.GotoTop()
}

func (m *Metadata) SetDryRunResult(msg sf.DeployRunDoneMsg) {
	var b strings.Builder
	header := "Deploy result"
	if msg.DryRun {
		header = "Dry-run result"
	}
	fmt.Fprintln(&b, metaSection.Render(header))
	if msg.Err != "" {
		fmt.Fprintln(&b, metaErr.Render(msg.Err))
	}
	if msg.Output != "" {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, msg.Output)
	}
	m.Viewport.SetContent(b.String())
	m.Viewport.GotoTop()
}

func (m *Metadata) SetStatus(s string) {
	m.Viewport.SetContent(lipgloss.NewStyle().Faint(true).Render(s))
}

func (m Metadata) Loaded() bool { return m.loaded }

var (
	metaHeader  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	metaSection = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
	metaDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	metaAdd     = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	metaDel     = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	metaWarn    = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	metaErr     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

func actionGlyph(action string) string {
	switch action {
	case "Deploy":
		return metaAdd.Render("+")
	case "Delete":
		return metaDel.Render("-")
	case "Conflict":
		return metaWarn.Render("!")
	case "Ignored":
		return metaDim.Render("·")
	}
	return " "
}

func renderMetaSummary(msg sf.DeployPreviewLoadedMsg) string {
	var b strings.Builder
	fmt.Fprintln(&b, metaHeader.Render("Deploy preview"))
	fmt.Fprintln(&b, metaDim.Render(msg.ProjectDir))
	fmt.Fprintln(&b)
	if msg.RawError != "" {
		fmt.Fprintln(&b, metaErr.Render(msg.RawError))
		return b.String()
	}
	counts := map[string]int{}
	for _, it := range msg.Items {
		counts[it.Type]++
	}
	fmt.Fprintf(&b, "  %s  %d to deploy\n", metaAdd.Render("+"), counts["Deploy"])
	fmt.Fprintf(&b, "  %s  %d to delete\n", metaDel.Render("-"), counts["Delete"])
	fmt.Fprintf(&b, "  %s  %d conflict(s)\n", metaWarn.Render("!"), counts["Conflict"])
	fmt.Fprintf(&b, "  %s  %d ignored\n", metaDim.Render("·"), counts["Ignored"])
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, metaDim.Render("ctrl+r runs a dry-run deploy. ctrl+d runs a real deploy (asks first)."))
	return b.String()
}

func (m Metadata) UpdateList(msg tea.Msg) (Metadata, tea.Cmd) {
	var cmd tea.Cmd
	m.List, cmd = m.List.Update(msg)
	return m, cmd
}

func (m Metadata) UpdateViewport(msg tea.Msg) (Metadata, tea.Cmd) {
	if vpJumpKeys(&m.Viewport, msg) {
		return m, nil
	}
	var cmd tea.Cmd
	m.Viewport, cmd = m.Viewport.Update(msg)
	return m, cmd
}

func (m Metadata) View(listFocus, detailFocus bool) string {
	left := BorderFor(listFocus).Render(m.List.View())
	right := BorderFor(detailFocus).Render(m.Viewport.View())
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

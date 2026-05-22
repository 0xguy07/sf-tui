package panes

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/0xguy07/sf-tui/sf"
)

type Limits struct {
	Viewport viewport.Model
	width    int
	limits   []sf.Limit
}

func NewLimits(width, height int) Limits {
	v := viewport.New(width, height)
	v.SetContent(lipgloss.NewStyle().Faint(true).Render("Press ctrl+r to load limits for the selected org."))
	return Limits{Viewport: v, width: width}
}

func (l *Limits) SetSize(width, height int) {
	l.width = width
	l.Viewport.Width = width
	l.Viewport.Height = height
	if len(l.limits) > 0 {
		l.Viewport.SetContent(l.render())
	}
}

func (l *Limits) SetLimits(limits []sf.Limit) {
	l.limits = limits
	l.Viewport.SetContent(l.render())
	l.Viewport.GotoTop()
}

func (l *Limits) Clear() {
	l.limits = nil
	l.Viewport.SetContent("")
}

var (
	limitNameStyle = lipgloss.NewStyle().Bold(true)
	limitDimStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	limitGreen     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	limitYellow    = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	limitRed       = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

func (l Limits) render() string {
	if len(l.limits) == 0 {
		return limitDimStyle.Render("(no limits loaded)")
	}

	// Find the longest name so columns line up.
	nameW := 0
	for _, lim := range l.limits {
		if n := len(lim.Name); n > nameW {
			nameW = n
		}
	}
	if nameW > 40 {
		nameW = 40
	}

	// Reserve space: name(nameW) + space + bar(barW) + space + "999%" + space + "used / max"
	barW := l.width - nameW - 28
	if barW < 10 {
		barW = 10
	}

	var b strings.Builder
	for _, lim := range l.limits {
		name := lim.Name
		if len(name) > nameW {
			name = name[:nameW-1] + "…"
		}
		pct := lim.Pct()
		bar := renderBar(barW, pct)
		pctStr := fmt.Sprintf("%3d%%", int(pct*100))
		usage := fmt.Sprintf("%s / %s", humanInt(lim.Used()), humanInt(lim.Max))

		fmt.Fprintf(&b, "%-*s  %s  %s  %s\n",
			nameW,
			limitNameStyle.Render(name),
			bar,
			pctStr,
			limitDimStyle.Render(usage),
		)
	}
	return b.String()
}

func renderBar(width int, pct float64) string {
	if width < 2 {
		width = 2
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	filled := int(pct * float64(width))
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	switch {
	case pct >= 0.9:
		return limitRed.Render(bar)
	case pct >= 0.7:
		return limitYellow.Render(bar)
	default:
		return limitGreen.Render(bar)
	}
}

func humanInt(n int) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", float64(n)/1_000_000_000)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 10_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func (l Limits) Update(msg tea.Msg) (Limits, tea.Cmd) {
	if vpJumpKeys(&l.Viewport, msg) {
		return l, nil
	}
	var cmd tea.Cmd
	l.Viewport, cmd = l.Viewport.Update(msg)
	return l, cmd
}

func (l Limits) View() string { return l.Viewport.View() }

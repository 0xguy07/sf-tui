package panes

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/0xguy07/sf-tui/sf"
)

type Logs struct {
	Viewport  viewport.Model
	lines     []string
	Running   bool
	maxLines  int
	Inspector bool // when true, viewport shows the structured tree
}

func NewLogs(width, height int) Logs {
	v := viewport.New(width, height)
	v.SetContent(lipgloss.NewStyle().Faint(true).Render("Press ctrl+l to start tailing Apex logs for the selected org."))
	return Logs{Viewport: v, maxLines: 2000}
}

func (l *Logs) SetSize(width, height int) {
	l.Viewport.Width = width
	l.Viewport.Height = height
}

func (l *Logs) Append(line string) {
	l.lines = append(l.lines, line)
	if len(l.lines) > l.maxLines {
		l.lines = l.lines[len(l.lines)-l.maxLines:]
	}
	if l.Inspector {
		l.refreshInspector()
	} else {
		l.Viewport.SetContent(strings.Join(l.lines, "\n"))
		l.Viewport.GotoBottom()
	}
}

func (l *Logs) Clear() {
	l.lines = nil
	l.Viewport.SetContent("")
}

// ToggleInspector flips the view; rebuilds content immediately.
func (l *Logs) ToggleInspector() {
	l.Inspector = !l.Inspector
	if l.Inspector {
		l.refreshInspector()
	} else {
		l.Viewport.SetContent(strings.Join(l.lines, "\n"))
		l.Viewport.GotoBottom()
	}
}

func (l *Logs) refreshInspector() {
	events := sf.ParseApexEvents(l.lines)
	l.Viewport.SetContent(renderInspector(events))
	l.Viewport.GotoTop()
}

var (
	logTime    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	logKind    = lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	logDebug   = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	logErr     = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	logEnter   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
)

func renderInspector(events []sf.ApexEvent) string {
	if len(events) == 0 {
		return lipgloss.NewStyle().Faint(true).Render("(no events parsed yet — start tailing or run apex)")
	}
	var b strings.Builder
	for _, e := range events {
		indent := strings.Repeat("  ", e.Depth)
		kindStyle := logKind
		switch {
		case e.Kind == "USER_DEBUG":
			kindStyle = logDebug
		case e.Kind == "EXCEPTION_THROWN", e.Kind == "FATAL_ERROR":
			kindStyle = logErr
		case e.IsEnter:
			kindStyle = logEnter
		}
		fmt.Fprintf(&b, "%s %s%s %s\n",
			logTime.Render(e.Time),
			indent,
			kindStyle.Render(e.Kind),
			truncateOne(e.Detail, 100),
		)
	}
	return b.String()
}

func truncateOne(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > max {
		return s[:max-1] + "…"
	}
	return s
}

func (l *Logs) SetStatus(msg string) {
	l.Viewport.SetContent(lipgloss.NewStyle().Faint(true).Render(msg))
}

func (l Logs) Update(msg tea.Msg) (Logs, tea.Cmd) {
	if vpJumpKeys(&l.Viewport, msg) {
		return l, nil
	}
	var cmd tea.Cmd
	l.Viewport, cmd = l.Viewport.Update(msg)
	return l, cmd
}

func (l Logs) View() string { return l.Viewport.View() }

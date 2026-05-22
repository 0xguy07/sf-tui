package panes

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Apex struct {
	Area     textarea.Model
	Viewport viewport.Model
	output   string
	Running  bool
}

func NewApex(width, height int) Apex {
	ta := textarea.New()
	ta.Placeholder = "System.debug('Hello, ' + UserInfo.getName());"
	ta.SetWidth(width)
	ta.SetHeight(height)
	ta.ShowLineNumbers = true
	ta.CharLimit = 16000

	vp := viewport.New(width, height)
	vp.SetContent(lipgloss.NewStyle().Faint(true).Render("Press ctrl+r to execute anonymous Apex against the selected org."))

	return Apex{Area: ta, Viewport: vp}
}

func (a *Apex) SetSize(width, editorH, outputH int) {
	a.Area.SetWidth(width)
	a.Area.SetHeight(editorH)
	a.Viewport.Width = width
	a.Viewport.Height = outputH
}

func (a *Apex) Focus() tea.Cmd { return a.Area.Focus() }
func (a *Apex) Blur()          { a.Area.Blur() }
func (a Apex) Value() string   { return a.Area.Value() }

func (a *Apex) SetOutput(s string) {
	a.output = s
	a.Viewport.SetContent(s)
	a.Viewport.GotoTop()
}

func (a Apex) Output() string { return a.output }

func (a *Apex) Clear() {
	a.output = ""
	a.Viewport.SetContent("")
}

// UpdateEditor routes a Msg to the textarea (when editor is focused).
func (a Apex) UpdateEditor(msg tea.Msg) (Apex, tea.Cmd) {
	var cmd tea.Cmd
	a.Area, cmd = a.Area.Update(msg)
	return a, cmd
}

// UpdateViewport routes a Msg to the output viewport (when output is focused).
func (a Apex) UpdateViewport(msg tea.Msg) (Apex, tea.Cmd) {
	if vpJumpKeys(&a.Viewport, msg) {
		return a, nil
	}
	var cmd tea.Cmd
	a.Viewport, cmd = a.Viewport.Update(msg)
	return a, cmd
}

func (a Apex) EditorView() string { return a.Area.View() }
func (a Apex) OutputView() string { return a.Viewport.View() }

// FormatApexResult turns the structured response into a human-readable block:
// status header, exception (if any), then debug log.
func FormatApexResult(success, compiled bool, compileProblem, exceptionMsg, exceptionStack, logs string) string {
	var b strings.Builder

	headerOK := lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	headerErr := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	switch {
	case !compiled:
		b.WriteString(headerErr.Render("✗ compile failed"))
		b.WriteString("\n")
		if compileProblem != "" {
			b.WriteString(compileProblem)
			b.WriteString("\n")
		}
	case !success:
		b.WriteString(headerErr.Render("✗ runtime error"))
		b.WriteString("\n")
		if exceptionMsg != "" {
			b.WriteString(exceptionMsg)
			b.WriteString("\n")
		}
		if exceptionStack != "" {
			b.WriteString(dim.Render(exceptionStack))
			b.WriteString("\n")
		}
	default:
		b.WriteString(headerOK.Render("✓ executed"))
		b.WriteString("\n")
	}

	if logs != "" {
		b.WriteString("\n")
		b.WriteString(dim.Render("── debug log ──"))
		b.WriteString("\n")
		b.WriteString(logs)
	}
	return b.String()
}

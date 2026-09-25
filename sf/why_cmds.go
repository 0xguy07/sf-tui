package sf

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// Seq on each Why message lets the pane drop results from a superseded run.

type WhyResolvedMsg struct {
	Seq   int
	Force bool
	Trace Trace
	Err   error
}

type WhyHistoryMsg struct {
	Seq  int
	Part HistoryPart
}

type WhyAutomationMsg struct {
	Seq  int
	Part AutomationPart
}

func WhyResolve(org, id string, force bool, seq int) tea.Cmd {
	return func() tea.Msg {
		if org == "" {
			return WhyResolvedMsg{Seq: seq, Err: fmt.Errorf("no org selected")}
		}
		t, err := ResolveRecord(RESTFor(org), org, id, force)
		return WhyResolvedMsg{Seq: seq, Force: force, Trace: t, Err: err}
	}
}

func WhyLoadHistory(org string, t Trace, force bool, seq int) tea.Cmd {
	return func() tea.Msg {
		return WhyHistoryMsg{Seq: seq, Part: LoadHistory(RESTFor(org), org, t, force)}
	}
}

func WhyLoadAutomation(org string, o ObjectRef, force bool, seq int) tea.Cmd {
	return func() tea.Msg {
		return WhyAutomationMsg{Seq: seq, Part: LoadAutomation(RESTFor(org), org, o, force)}
	}
}

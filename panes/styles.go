package panes

import "github.com/charmbracelet/lipgloss"

var (
	activeBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("205"))
	idleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240"))
)

func BorderFor(active bool) lipgloss.Style {
	if active {
		return activeBorder
	}
	return idleBorder
}

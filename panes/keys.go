package panes

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// vpJumpKeys handles g/G/home/end on a viewport before delegating to
// viewport.Update. Returns (handled, vp). If handled is true the caller should
// not pass msg to viewport.Update.
func vpJumpKeys(vp *viewport.Model, msg tea.Msg) bool {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return false
	}
	switch km.String() {
	case "g", "home":
		vp.GotoTop()
		return true
	case "G", "end":
		vp.GotoBottom()
		return true
	}
	return false
}

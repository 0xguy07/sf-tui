package sf

import (
	"fmt"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
)

type OpenedMsg struct {
	What string // human description for status line
}

// OpenInOrg shells out to `sf org open --path` to launch a browser tab
// at the given path within the org. Path should begin with "/".
func OpenInOrg(orgAliasOrUser, path, what string) tea.Cmd {
	return func() tea.Msg {
		if orgAliasOrUser == "" {
			return ErrMsg{Err: fmt.Errorf("no org selected")}
		}
		args := []string{"org", "open", "--target-org", orgAliasOrUser}
		if path != "" {
			args = append(args, "--path", path)
		}
		cmd := exec.Command("sf", args...)
		if err := cmd.Run(); err != nil {
			if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
				return ErrMsg{Err: fmt.Errorf("%s", string(ee.Stderr))}
			}
			return ErrMsg{Err: fmt.Errorf("sf org open: %w", err)}
		}
		return OpenedMsg{What: what}
	}
}

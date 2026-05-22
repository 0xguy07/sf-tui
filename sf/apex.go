package sf

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
)

type ApexDoneMsg struct {
	Success         bool
	Compiled        bool
	CompileProblem  string
	ExceptionMsg    string
	ExceptionStack  string
	Logs            string
}

type apexRunResult struct {
	Status int `json:"status"`
	Result struct {
		Success             bool   `json:"success"`
		Compiled            bool   `json:"compiled"`
		CompileProblem      string `json:"compileProblem"`
		ExceptionMessage    string `json:"exceptionMessage"`
		ExceptionStackTrace string `json:"exceptionStackTrace"`
		Logs                string `json:"logs"`
	} `json:"result"`
	Message string `json:"message,omitempty"`
}

func RunApex(orgAliasOrUser, code string) tea.Cmd {
	return func() tea.Msg {
		if orgAliasOrUser == "" {
			return ErrMsg{Err: fmt.Errorf("no org selected")}
		}
		f, err := os.CreateTemp("", "sf-tui-apex-*.apex")
		if err != nil {
			return ErrMsg{Err: fmt.Errorf("create temp: %w", err)}
		}
		path := f.Name()
		defer os.Remove(path)
		if _, err := f.WriteString(code); err != nil {
			f.Close()
			return ErrMsg{Err: fmt.Errorf("write temp: %w", err)}
		}
		f.Close()

		cmd := exec.Command("sf", "apex", "run",
			"--target-org", orgAliasOrUser,
			"--file", path,
			"--json",
		)
		out, err := cmd.Output()
		if err != nil {
			// `sf apex run` returns a non-zero exit code on compile/runtime errors,
			// but still writes the structured result to stdout. Try to parse before
			// surfacing the raw error.
			var r apexRunResult
			if jerr := json.Unmarshal(out, &r); jerr == nil && (r.Result.CompileProblem != "" || r.Result.ExceptionMessage != "" || r.Result.Logs != "") {
				return apexMsgFromResult(r)
			}
			if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
				return ErrMsg{Err: fmt.Errorf("%s", string(ee.Stderr))}
			}
			return ErrMsg{Err: fmt.Errorf("sf apex run: %w", err)}
		}
		var r apexRunResult
		if err := json.Unmarshal(out, &r); err != nil {
			return ErrMsg{Err: fmt.Errorf("parse apex result: %w", err)}
		}
		return apexMsgFromResult(r)
	}
}

func apexMsgFromResult(r apexRunResult) ApexDoneMsg {
	return ApexDoneMsg{
		Success:        r.Result.Success,
		Compiled:       r.Result.Compiled,
		CompileProblem: r.Result.CompileProblem,
		ExceptionMsg:   r.Result.ExceptionMessage,
		ExceptionStack: r.Result.ExceptionStackTrace,
		Logs:           r.Result.Logs,
	}
}

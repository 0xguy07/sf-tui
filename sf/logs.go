package sf

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
)

type LogLineMsg struct {
	Line string
}

type LogEndedMsg struct {
	Err error
}

type LogTail struct {
	cancel context.CancelFunc
	lines  chan string
	done   chan error
}

func StartLogTail(orgAliasOrUser string) (*LogTail, tea.Cmd, error) {
	if orgAliasOrUser == "" {
		return nil, nil, fmt.Errorf("no org selected")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "sf", "apex", "tail", "log",
		"--target-org", orgAliasOrUser,
		"--debug-level", "SFDC_DevConsole",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, nil, err
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, nil, err
	}

	lines := make(chan string, 64)
	done := make(chan error, 1)
	tail := &LogTail{cancel: cancel, lines: lines, done: done}

	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			select {
			case lines <- scanner.Text():
			case <-ctx.Done():
				return
			}
		}
		done <- cmd.Wait()
		close(lines)
	}()

	return tail, tail.Next(), nil
}

// Next returns a tea.Cmd that waits for the next log line or end of stream.
func (t *LogTail) Next() tea.Cmd {
	return func() tea.Msg {
		select {
		case line, ok := <-t.lines:
			if !ok {
				return LogEndedMsg{Err: <-t.done}
			}
			return LogLineMsg{Line: line}
		}
	}
}

func (t *LogTail) Stop() {
	if t.cancel != nil {
		t.cancel()
	}
}

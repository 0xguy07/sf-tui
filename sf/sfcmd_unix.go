//go:build !windows

package sf

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// killGroup makes the sf process the leader of its own process group and,
// on deadline, kills the whole group. sf is a Node program that may spawn
// children; killing only the parent would leave them holding our pipes open.
func killGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			// The group already exited at the deadline: not a timeout.
			return os.ErrProcessDone
		}
		return err
	}
}

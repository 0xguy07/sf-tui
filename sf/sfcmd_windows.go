//go:build windows

package sf

import "os/exec"

// killGroup is a no-op on Windows: CommandContext's default kill applies.
func killGroup(cmd *exec.Cmd) {}

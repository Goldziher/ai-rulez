//go:build !windows

package evals

import (
	"os/exec"
	"syscall"
)

// isolateProcessGroup puts the command in its own process group and makes a
// cancellation kill the whole group, so a shell the harness started dies with it.
func isolateProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

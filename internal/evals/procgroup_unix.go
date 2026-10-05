//go:build !windows

package evals

import (
	"os/exec"
	"syscall"
	"time"
)

// killTreeOnCancel runs cmd in its own process group and, when its context ends,
// kills the whole group, so background children of a runner or assertion do not
// outlive the timeout.
func killTreeOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 2 * time.Second
}

// killTree kills the process group of a started cmd.
func killTree(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) //nolint:errcheck // the group may already be gone
	}
}

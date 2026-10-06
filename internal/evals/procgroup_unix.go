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
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			// The group is already gone (ESRCH) or cannot be signalled: kill the
			// child itself so the cancel never reports a spurious failure.
			return cmd.Process.Kill() //nolint:wrapcheck // os.ErrProcessDone is handled by exec
		}
		return nil
	}
	cmd.WaitDelay = 2 * time.Second
}

// runTree starts cmd and waits for it.
func runTree(cmd *exec.Cmd) error { return cmd.Run() } //nolint:wrapcheck // the caller adds context

// killTree kills the process group of a started cmd.
func killTree(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) //nolint:errcheck // ESRCH when nothing is left
	}
}

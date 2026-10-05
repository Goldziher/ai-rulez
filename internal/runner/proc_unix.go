//go:build !windows

package runner

import (
	"os/exec"
	"syscall"
)

func isWindows() bool { return false }

// procTree owns the processes a command started.
type procTree struct{}

// configure puts the child in its own process group and makes cancellation kill
// the whole group, so a scanner's helpers die with it.
func configure(cmd *exec.Cmd) *procTree {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			return cmd.Process.Kill() //nolint:wrapcheck // the group may already be gone
		}
		return nil
	}
	return &procTree{}
}

func (*procTree) attach(*exec.Cmd) {}

// kill signals the whole process group; it is a no-op once the group is empty.
func (*procTree) kill(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) //nolint:errcheck // ESRCH when nothing is left
	}
}

func (*procTree) close() {}

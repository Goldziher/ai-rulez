//go:build windows

package telemetry

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// detach lets the opt-in background flush survive the npm launcher's job.
func detach(cmd *exec.Cmd) bool {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_BREAKAWAY_FROM_JOB}
	return true
}

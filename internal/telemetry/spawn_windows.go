//go:build windows

package telemetry

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"

	"github.com/Goldziher/ai-rulez/v5/internal/npmwatch"
)

// detach lets the opt-in background flush survive the npm launcher's job.
func detach(cmd *exec.Cmd) bool {
	if os.Getenv(npmwatch.ParentEnvironment) != npmwatch.JobMarker(os.Getpid()) {
		return false
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_BREAKAWAY_FROM_JOB}
	return true
}

//go:build unix

package telemetry

import (
	"os/exec"
	"syscall"
)

// detach makes the child a session leader so it survives the hook that started it.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

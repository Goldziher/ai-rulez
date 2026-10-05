//go:build windows

package evals

import (
	"os/exec"
	"time"
)

// killTreeOnCancel bounds the wait after a cancel; Windows has no process groups
// to signal here, so only the direct child is killed.
func killTreeOnCancel(cmd *exec.Cmd) {
	cmd.WaitDelay = 2 * time.Second
}

// killTree kills a started cmd.
func killTree(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill() //nolint:errcheck // the process may already have exited
	}
}

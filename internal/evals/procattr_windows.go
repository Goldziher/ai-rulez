//go:build windows

package evals

import "os/exec"

// isolateProcessGroup is a no-op on Windows: a cancellation kills the process.
func isolateProcessGroup(_ *exec.Cmd) {}

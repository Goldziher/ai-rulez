//go:build !unix && !windows

package telemetry

import "os/exec"

func detach(*exec.Cmd) bool { return false }

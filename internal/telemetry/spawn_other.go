//go:build !unix

package telemetry

import "os/exec"

func detach(*exec.Cmd) {}

//go:build windows

package commands

import "github.com/samber/oops"

// startDaemonProcess is never reached on Windows: the shared daemon is not
// offered there, so ServeViaDaemon falls back in-process before asking for a
// spawner. It exists so the command compiles.
func startDaemonProcess(string) error {
	return oops.Errorf("the shared MCP daemon is not supported on this platform")
}

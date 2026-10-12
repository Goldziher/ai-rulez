//go:build !windows

package commands

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/samber/oops"
)

// startDaemonProcess spawns the shared daemon: the same command line in a new
// session, with stdin, stdout and stderr on the null device so it never touches
// the MCP pipe. The child inherits the working directory and derives the same
// socket as its proxy. It is detached from the proxy's session, so it outlives
// the session that started it and serves later ones.
func startDaemonProcess(_ string) error {
	exe, err := os.Executable()
	if err != nil {
		return oops.Wrapf(err, "locate the ai-rulez binary to start the shared daemon")
	}
	args := append(append([]string(nil), os.Args[1:]...), "--"+flagServeDaemonServer)
	cmd := exec.Command(exe, args...) //nolint:gosec // the binary's own command line
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return oops.Wrapf(err, "start the shared daemon")
	}
	// Reap the child when it exits, so it does not become a zombie while the
	// proxy is still alive; if the proxy exits first, init adopts it.
	go func() { _ = cmd.Wait() }() //nolint:errcheck // the exit status is not the proxy's concern
	return nil
}
